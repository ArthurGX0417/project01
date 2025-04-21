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
	PricePerHalfHour float64             `json:"price_per_half_hour" binding:"gte=0"` // 恢復 PricePerHalfHour
	DailyMaxPrice    float64             `json:"daily_max_price" binding:"gte=0"`
	MonthlyPrice     float64             `json:"monthly_price" binding:"gte=0"` // 新增 MonthlyPrice
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
		PricePerHalfHour: input.PricePerHalfHour, // 恢復 PricePerHalfHour
		DailyMaxPrice:    input.DailyMaxPrice,
		MonthlyPrice:     input.MonthlyPrice, // 新增 MonthlyPrice
		Longitude:        input.Longitude,
		Latitude:         input.Latitude,
		Status:           "available",
	}

	if spot.PricePerHalfHour == 0 && spot.PricingType == "hourly" {
		spot.PricePerHalfHour = 20.00
	}
	if spot.DailyMaxPrice == 0 && spot.PricingType == "hourly" {
		spot.DailyMaxPrice = 300.00
	}
	if spot.MonthlyPrice == 0 && spot.PricingType == "monthly" {
		spot.MonthlyPrice = 5000.00
	}

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

	if err := services.ShareParkingSpot(spot, availableDays); err != nil {
		log.Printf("Failed to share parking spot for member %d: %v", input.MemberID, err)
		ErrorResponse(c, http.StatusInternalServerError, "儲存車位失敗", err.Error())
		return
	}

	refreshedSpot, availableDaysFetched, err := services.GetParkingSpotByID(spot.SpotID)
	if err != nil {
		log.Printf("Failed to refresh parking spot with ID %d: %v", spot.SpotID, err)
		ErrorResponse(c, http.StatusInternalServerError, "刷新車位資料失敗", err.Error())
		return
	}

	var rents []models.Rent
	if err := database.DB.Where("spot_id = ?", refreshedSpot.SpotID).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", refreshedSpot.SpotID, err)
		rents = []models.Rent{}
	}

	SuccessResponse(c, http.StatusOK, "車位共享成功", refreshedSpot.ToResponse(availableDaysFetched, rents))
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

	startOfDay := time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Nanosecond)

	var rentedSpotIDs []int
	if err := database.DB.Model(&models.Rent{}).
		Select("spot_id").
		Where("(actual_end_time IS NULL AND end_time >= ?) OR (end_time > ? AND start_time < ?)", now, startOfDay, endOfDay).
		Distinct().
		Scan(&rentedSpotIDs).Error; err != nil {
		log.Printf("Failed to query rented spot IDs: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "failed to query rented spot IDs: database error",
		})
		return
	}
	log.Printf("Rented spot IDs: %v", rentedSpotIDs)

	var parkingSpots []models.ParkingSpot
	query := database.DB.
		Preload("Member").
		Preload("Rents", "end_time >= ? OR actual_end_time IS NULL", now).
		Preload("AvailableDays", "DATE(available_date) = ? AND is_available = ?", dateStr, true).
		Where("status = ?", "available")

	if len(rentedSpotIDs) > 0 {
		query = query.Where("spot_id NOT IN (?)", rentedSpotIDs)
	}

	if err := query.Find(&parkingSpots).Error; err != nil {
		log.Printf("Failed to get parking spots: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "failed to get parking spots: database error",
		})
		return
	}

	log.Printf("Queried %d parking spots for date %s:", len(parkingSpots), dateStr)
	for _, spot := range parkingSpots {
		log.Printf("Spot %d: Status=%s, AvailableDays=%v", spot.SpotID, spot.Status, spot.AvailableDays)
	}

	var availableSpots []models.ParkingSpot
	unavailableSpots := []int{}
	for _, spot := range parkingSpots {
		if len(spot.AvailableDays) > 0 {
			availableSpots = append(availableSpots, spot)
			log.Printf("Spot %d included in available spots", spot.SpotID)
		} else {
			unavailableSpots = append(unavailableSpots, spot.SpotID)
			log.Printf("Spot %d excluded: No available days for date %s", spot.SpotID, dateStr)
		}
	}

	log.Printf("Found %d parking spots, %d available after filtering for date %s", len(parkingSpots), len(availableSpots), dateStr)

	if len(availableSpots) == 0 {
		message := fmt.Sprintf("所選條件（日期：%s）目前沒有符合的車位！請調整篩選條件。", dateStr)
		if len(unavailableSpots) > 0 {
			message += fmt.Sprintf(" 以下車位未設定可用日期：%v", unavailableSpots)
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": message,
			"data":    []models.ParkingSpot{},
		})
		return
	}

	availableSpotsResponse := make([]models.ParkingSpotResponse, len(availableSpots))
	for i, spot := range availableSpots {
		availableSpotsResponse[i] = spot.ToResponse(spot.AvailableDays, spot.Rents)
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

	// 查詢該停車位的租賃記錄
	var rents []models.Rent
	if err := database.DB.Where("spot_id = ?", spot.SpotID).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", spot.SpotID, err)
		rents = []models.Rent{}
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", spot.ToResponse(availableDays, rents))
}

func UpdateParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的車位ID", err.Error())
		return
	}

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

	if spot.MemberID != currentMemberIDInt {
		log.Printf("Member %d attempted to update parking spot %d owned by member %d", currentMemberIDInt, id, spot.MemberID)
		ErrorResponse(c, http.StatusForbidden, "無權限", "you are not the owner of this parking spot")
		return
	}

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

	// 查詢該停車位的租賃記錄
	var rents []models.Rent
	if err := database.DB.Where("spot_id = ?", updatedSpot.SpotID).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", updatedSpot.SpotID, err)
		rents = []models.Rent{}
	}

	SuccessResponse(c, http.StatusOK, "車位更新成功", updatedSpot.ToResponse(availableDays, rents))
}

// GetParkingSpotIncome 查看指定車位的收入（僅限 shared_owner）
func GetParkingSpotIncome(c *gin.Context) {
	// 獲取車位 ID
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的車位 ID", "parking spot ID must be a number", "ERR_INVALID_SPOT_ID")
		return
	}

	// 獲取當前會員 ID 和角色
	currentMemberID, exists := c.Get("member_id")
	if !exists {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token", "ERR_NO_MEMBER_ID")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type", "ERR_INVALID_MEMBER_ID_TYPE")
		return
	}

	// 查詢車位
	var spot models.ParkingSpot
	if err := database.DB.Preload("Rents").First(&spot, id).Error; err != nil {
		log.Printf("Failed to find parking spot %d: %v", id, err)
		ErrorResponse(c, http.StatusNotFound, "車位不存在", "parking spot not found", "ERR_SPOT_NOT_FOUND")
		return
	}

	// 檢查車位是否屬於當前會員
	if spot.MemberID != currentMemberIDInt {
		ErrorResponse(c, http.StatusForbidden, "無權限", "you can only view income of your own parking spot", "ERR_INSUFFICIENT_PERMISSIONS")
		return
	}

	// 計算總收入
	var totalIncome float64
	for _, rent := range spot.Rents {
		if rent.TotalCost > 0 { // 確保只計算已結算的租賃
			totalIncome += rent.TotalCost
		}
	}

	// 返回收入數據
	response := gin.H{
		"spot_id":      spot.SpotID,
		"location":     spot.Location,
		"total_income": totalIncome,
	}

	SuccessResponse(c, http.StatusOK, "查詢車位收入成功", response)
}
