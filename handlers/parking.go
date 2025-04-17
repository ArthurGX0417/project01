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
		Status:           "available", // 初始狀態為 available
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
		parsedDate, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			log.Printf("Invalid date format for available_days: %v", err)
			ErrorResponse(c, http.StatusBadRequest, "無效的日期格式", "date must be in YYYY-MM-DD format")
			return
		}
		availableDays[i] = models.ParkingSpotAvailableDay{
			AvailableDate: parsedDate,
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
	dateStr := c.Query("date")
	log.Printf("Received request with date: %s", dateStr)
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "請提供 date 參數（格式為 YYYY-MM-DD）",
		})
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		log.Printf("Invalid date format: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   fmt.Sprintf("無效的日期格式，應為 YYYY-MM-DD: %v", err),
		})
		return
	}

	parsedDateStr := date.Format("2006-01-02")
	if parsedDateStr != dateStr {
		log.Printf("Invalid date (e.g., 2025-02-30 does not exist)")
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "無效的日期（例如 2025-02-30 不存在）",
		})
		return
	}

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if date.Before(today) {
		log.Printf("Date must be today or in the future: %s", dateStr)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "日期必須為今天或未來的日期",
		})
		return
	}

	var parkingSpots []models.ParkingSpot
	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Nanosecond)

	subQuery := database.DB.Model(&models.Rent{}).
		Select("spot_id").
		Where("(actual_end_time IS NULL AND end_time >= ?) OR (end_time > ? AND start_time < ?)", now, startOfDay, endOfDay)

	if err := database.DB.
		Preload("Member").
		Preload("Rents", "end_time >= ? OR actual_end_time IS NULL", now).
		Preload("AvailableDays", "DATE(available_date) = ? AND is_available = ?", dateStr, true).
		Where("status = ? AND NOT EXISTS (?)", "available", subQuery).
		Find(&parkingSpots).Error; err != nil {
		log.Printf("Failed to get parking spots: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "failed to get parking spots: database error",
		})
		return
	}

	// 添加日誌：記錄查詢到的所有車位
	log.Printf("Queried %d parking spots for date %s:", len(parkingSpots), dateStr)
	for _, spot := range parkingSpots {
		log.Printf("Spot %d: Status=%s, AvailableDays=%v", spot.SpotID, spot.Status, spot.AvailableDays)
	}

	var availableSpots []models.ParkingSpot
	for _, spot := range parkingSpots {
		if len(spot.AvailableDays) > 0 {
			availableSpots = append(availableSpots, spot)
			log.Printf("Spot %d included in available spots", spot.SpotID)
		} else {
			log.Printf("Spot %d excluded: No available days for date %s", spot.SpotID, dateStr)
		}
	}

	log.Printf("Found %d parking spots, %d available after filtering for date %s", len(parkingSpots), len(availableSpots), dateStr)

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

	// 檢查車位是否存在並獲取車位資訊
	spot, _, err := services.GetParkingSpotByID(id)
	if err != nil {
		log.Printf("Failed to get parking spot: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢車位失敗", err.Error())
		return
	}
	if spot == nil {
		ErrorResponse(c, http.StatusNotFound, "車位不存在", "parking spot not found")
		return
	}

	// 檢查當前會員是否為車位的擁有者
	if spot.MemberID != currentMemberIDInt {
		log.Printf("Member %d attempted to update parking spot %d owned by member %d", currentMemberIDInt, id, spot.MemberID)
		ErrorResponse(c, http.StatusForbidden, "無權限", "you are not the owner of this parking spot")
		return
	}

	// 檢查會員角色是否為 shared_owner
	member, err := services.GetMemberByID(currentMemberIDInt)
	if err != nil {
		log.Printf("Failed to get member: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢會員失敗", err.Error())
		return
	}
	if member == nil {
		ErrorResponse(c, http.StatusNotFound, "會員不存在", "member not found")
		return
	}
	if member.Role != "shared_owner" {
		log.Printf("Member %d with role %s attempted to update parking spot %d", currentMemberIDInt, member.Role, id)
		ErrorResponse(c, http.StatusForbidden, "無權限", "only shared_owner can update parking spots")
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
