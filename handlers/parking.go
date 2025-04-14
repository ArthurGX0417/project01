package handlers

import (
	"fmt"
	"log"
	"net/http"
	"project01/database"
	"project01/models"
	"project01/services"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type AvailableDayInput struct {
	Date        string `json:"date" binding:"required,datetime=2006-01-02"`
	IsAvailable bool   `json:"is_available"`
}

type ParkingSpotInput struct {
	MemberID         int                 `json:"member_id" binding:"required,gt=0"`
	ParkingType      string              `json:"parking_type" binding:"required,oneof=mechanical flat"`
	FloorLevel       string              `json:"floor_level" binding:"omitempty,max=20"`
	Location         string              `json:"location" binding:"required,max=50"`
	PricingType      string              `json:"pricing_type" binding:"required,oneof=monthly hourly"`
	PricePerHalfHour float64             `json:"price_per_half_hour" binding:"gte=0"`
	DailyMaxPrice    float64             `json:"daily_max_price" binding:"gte=0"`
	Longitude        float64             `json:"longitude" binding:"gte=-180,lte=180"`
	Latitude         float64             `json:"latitude" binding:"gte=-90,lte=90"`
	AvailableDays    []AvailableDayInput `json:"available_days" binding:"omitempty,dive"`
}

func ShareParkingSpot(c *gin.Context) {
	var input ParkingSpotInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	// 從 token 中提取當前用戶的 member_id
	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type")
		return
	}

	// 檢查請求中的 member_id 是否與當前用戶一致
	if currentMemberIDInt != input.MemberID {
		log.Printf("Member %d attempted to share parking spot for member %d", currentMemberIDInt, input.MemberID)
		ErrorResponse(c, http.StatusForbidden, "無權限", "you can only share parking spots for yourself")
		return
	}

	spot := &models.ParkingSpot{
		MemberID:         input.MemberID,
		ParkingType:      input.ParkingType,
		FloorLevel:       input.FloorLevel,
		Location:         input.Location,
		PricingType:      input.PricingType,
		PricePerHalfHour: input.PricePerHalfHour,
		DailyMaxPrice:    input.DailyMaxPrice,
		Longitude:        input.Longitude,
		Latitude:         input.Latitude,
		Status:           "idle",
	}

	// 設置預設價格
	if spot.PricePerHalfHour == 0 {
		spot.PricePerHalfHour = 20.00
	}
	if spot.DailyMaxPrice == 0 {
		spot.DailyMaxPrice = 300.00
	}

	// 處理可用日期
	availableDays := make([]models.ParkingSpotAvailableDay, len(input.AvailableDays))
	for i, day := range input.AvailableDays {
		// 將 string 類型的日期解析為 time.Time
		parsedDate, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			log.Printf("Invalid date format for available_days: %v", err)
			ErrorResponse(c, http.StatusBadRequest, "無效的日期格式", "date must be in YYYY-MM-DD format")
			return
		}
		availableDays[i] = models.ParkingSpotAvailableDay{
			AvailableDate: parsedDate, // 使用解析後的 time.Time
			IsAvailable:   day.IsAvailable,
		}
	}

	// 調用服務層創建停車位
	if err := services.ShareParkingSpot(spot, availableDays); err != nil {
		log.Printf("Failed to share parking spot for member %d: %v", input.MemberID, err)
		ErrorResponse(c, http.StatusInternalServerError, "儲存車位失敗", err.Error())
		return
	}

	// 刷新停車位資料
	refreshedSpot, availableDaysFetched, err := services.GetParkingSpotByID(spot.SpotID)
	if err != nil {
		log.Printf("Failed to refresh parking spot with ID %d: %v", spot.SpotID, err)
		ErrorResponse(c, http.StatusInternalServerError, "刷新車位資料失敗", err.Error())
		return
	}

	SuccessResponse(c, http.StatusOK, "車位共享成功", refreshedSpot.ToResponse(availableDaysFetched))
}

func GetAvailableParkingSpots(c *gin.Context) {
	// 獲取日期參數
	dateStr := c.Query("date")
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "請提供日期參數",
		})
		return
	}

	// 驗證日期格式（應為 YYYY-MM-DD）
	_, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   fmt.Sprintf("無效的日期格式: %v", err),
		})
		return
	}

	// 查詢所有車位，並預載入關聯資料
	var parkingSpots []models.ParkingSpot
	if err := database.DB.
		Preload("Member").
		Preload("Rents").
		Preload("AvailableDays", "available_date = ? AND is_available = ?", dateStr, true).
		Where("status = ?", "idle").
		Find(&parkingSpots).Error; err != nil {
		log.Printf("Failed to get parking spots: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   fmt.Sprintf("failed to get parking spots: %v", err),
		})
		return
	}

	// 過濾車位：只保留有可用日期的車位
	var availableSpots []models.ParkingSpot
	for _, spot := range parkingSpots {
		if len(spot.AvailableDays) > 0 { // 如果有符合條件的 AvailableDays
			availableSpots = append(availableSpots, spot)
		}
	}

	// 將結果轉換為回應格式
	availableSpotsResponse := make([]models.ParkingSpotResponse, len(availableSpots))
	for i, spot := range availableSpots {
		availableSpotsResponse[i] = spot.ToResponse(spot.AvailableDays)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    availableSpotsResponse,
	})
}

func GetParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的車位ID", err.Error())
		return
	}

	spot, availableDays, err := services.GetParkingSpotByID(id)
	if err != nil {
		log.Printf("Failed to get parking spot: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢車位失敗", err.Error())
		return
	}
	if spot == nil {
		ErrorResponse(c, http.StatusNotFound, "車位不存在", "parking spot not found")
		return
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", spot.ToResponse(availableDays))
}

func UpdateParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的車位ID", err.Error())
		return
	}

	var updatedFields map[string]interface{}
	if err := c.ShouldBindJSON(&updatedFields); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	if len(updatedFields) == 0 {
		ErrorResponse(c, http.StatusBadRequest, "未提供任何更新字段", "no fields provided for update")
		return
	}

	if err := services.UpdateParkingSpot(id, updatedFields); err != nil {
		log.Printf("Failed to update parking spot with ID %d: %v", id, err)
		if err.Error() == fmt.Sprintf("parking spot with ID %d not found", id) {
			ErrorResponse(c, http.StatusNotFound, "車位不存在", err.Error())
		} else {
			ErrorResponse(c, http.StatusInternalServerError, "車位更新失敗", err.Error())
		}
		return
	}

	updatedSpot, availableDays, err := services.GetParkingSpotByID(id)
	if err != nil {
		log.Printf("Failed to fetch updated parking spot with ID %d: %v", id, err)
		ErrorResponse(c, http.StatusInternalServerError, "獲取更新後的車位資料失敗", err.Error())
		return
	}

	SuccessResponse(c, http.StatusOK, "車位更新成功", updatedSpot.ToResponse(availableDays))
}
