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
	"gorm.io/gorm"
)

// RentInput 定義用於綁定請求的輸入結構體
type RentInput struct {
	SpotID    int    `json:"spot_id" binding:"required"`
	StartTime string `json:"start_time" binding:"required"`
	EndTime   string `json:"end_time" binding:"required"`
}

// parseTimeWithCST 解析時間字符串並轉換為 CST
func parseTimeWithCST(timeStr string) (time.Time, error) {
	// 嘗試解析 RFC 3339 格式（包含時區資訊）
	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		// 獲取時區偏移量
		_, offset := t.Zone()
		cstZone := time.FixedZone("CST", 8*60*60) // +08:00
		if offset == 0 {                          // 不帶時區，假設為 CST
			log.Printf("Parsed RFC3339 time without timezone %s, assumed as CST: %s", timeStr, t.In(cstZone).Format("2006-01-02T15:04:05"))
			return t.In(cstZone), nil
		}
		// 檢查時區是否為 +08:00 或 Z
		if offset != 8*60*60 && t.Location().String() != "Z" {
			return time.Time{}, fmt.Errorf("time zone must be +08:00 or Z, got offset %d hours", offset/(60*60))
		}
		log.Printf("Parsed RFC3339 time %s, converted to CST: %s", timeStr, t.In(cstZone).Format("2006-01-02T15:04:05"))
		return t.In(cstZone), nil
	}

	// 嘗試解析不帶時區的格式（假設為 CST）
	t, err = time.Parse("2006-01-02T15:04:05", timeStr)
	if err == nil {
		cstZone := time.FixedZone("CST", 8*60*60) // +08:00
		log.Printf("Parsed time %s as CST: %s", timeStr, t.In(cstZone).Format("2006-01-02T15:04:05"))
		return t.In(cstZone), nil
	}

	return time.Time{}, fmt.Errorf("time must be in 'YYYY-MM-DDThh:mm:ss' or RFC 3339 format")
}

// RentParkingSpot 租車位資料檢查
func RentParkingSpot(c *gin.Context) {
	var input RentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的輸入資料",
			"error":   "請提供車位 ID、開始時間和結束時間",
			"code":    "ERR_INVALID_INPUT",
		})
		return
	}

	// 解析開始時間和結束時間為 CST
	startTime, err := parseTimeWithCST(input.StartTime)
	if err != nil {
		log.Printf("Failed to parse start_time %s: %v", input.StartTime, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的開始時間格式",
			"error":   err.Error(),
			"code":    "ERR_INVALID_TIME_FORMAT",
		})
		return
	}

	endTime, err := parseTimeWithCST(input.EndTime)
	if err != nil {
		log.Printf("Failed to parse end_time %s: %v", input.EndTime, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的結束時間格式",
			"error":   err.Error(),
			"code":    "ERR_INVALID_TIME_FORMAT",
		})
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "member_id not found in token",
			"code":    "ERR_NO_MEMBER_ID",
		})
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid member_id type",
			"code":    "ERR_INVALID_MEMBER_ID",
		})
		return
	}

	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	log.Printf("Current CST time: %s, StartTime: %s", now.Format("2006-01-02T15:04:05"), startTime.Format("2006-01-02T15:04:05"))

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if startTime.Before(today) {
		log.Printf("Start time %s is before today %s", startTime.Format("2006-01-02T15:04:05"), today.Format("2006-01-02T15:04:05"))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "開始時間必須在今天或未來",
			"error":   "start_time must be today or in the future",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	if !endTime.After(startTime) {
		log.Printf("End time %s is not after start time %s", endTime.Format("2006-01-02T15:04:05"), startTime.Format("2006-01-02T15:04:05"))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "結束時間必須晚於開始時間",
			"error":   "end_time must be after start_time",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	var parkingSpot models.ParkingSpot
	if err := database.DB.Preload("Rents").First(&parkingSpot, input.SpotID).Error; err != nil {
		log.Printf("Failed to find parking spot %d: %v", input.SpotID, err)
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "車位不存在",
			"error":   "parking spot not found",
			"code":    "ERR_NOT_FOUND",
		})
		return
	}

	// 檢查是否有活躍租賃，並自動更新車位狀態
	hasActiveRent := false
	for _, rent := range parkingSpot.Rents {
		if rent.ActualEndTime == nil && rent.EndTime.After(now) {
			hasActiveRent = true
			log.Printf("Parking spot %d has an active rent: rent_id %d, end_time %s", input.SpotID, rent.RentID, rent.EndTime.Format("2006-01-02T15:04:05"))
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "車位已被租用",
				"error":   "parking spot has an active rent",
				"code":    "ERR_SPOT_ACTIVE_RENT",
			})
			return
		}
		if (startTime.Before(rent.EndTime) || startTime.Equal(rent.EndTime)) &&
			(endTime.After(rent.StartTime) || endTime.Equal(rent.StartTime)) {
			log.Printf("New rent time range overlaps with existing rent for spot %d: rent_id %d", input.SpotID, rent.RentID)
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "車位在指定時間範圍內不可用",
				"error":   "parking spot is not available for the specified time range",
				"code":    "ERR_TIME_OVERLAP",
			})
			return
		}
	}

	// 自動更新車位狀態
	var isDayAvailable bool
	startDate := startTime.Format("2006-01-02")
	var availableDayCount int64
	if err := database.DB.Model(&models.ParkingSpotAvailableDay{}).
		Where("parking_spot_id = ? AND available_date = ? AND is_available = ?", input.SpotID, startDate, true).
		Count(&availableDayCount).Error; err != nil {
		log.Printf("Failed to check available days for spot %d: %v", input.SpotID, err)
	}
	isDayAvailable = availableDayCount > 0

	if !hasActiveRent {
		if isDayAvailable && parkingSpot.Status != "available" {
			log.Printf("Resetting parking spot %d status to available", input.SpotID)
			parkingSpot.Status = "available"
		} else if !isDayAvailable && parkingSpot.Status != "occupied" {
			log.Printf("Resetting parking spot %d status to occupied", input.SpotID)
			parkingSpot.Status = "occupied"
		}
		if err := database.DB.Save(&parkingSpot).Error; err != nil {
			log.Printf("Failed to update parking spot status for spot %d: %v", input.SpotID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "更新車位狀態失敗",
				"error":   err.Error(),
				"code":    "ERR_UPDATE_STATUS",
			})
			return
		}
	}

	if parkingSpot.Status != "available" {
		log.Printf("Parking spot %d is not available", input.SpotID)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "車位不可用",
			"error":   "parking spot is not available",
			"code":    "ERR_SPOT_NOT_AVAILABLE",
		})
		return
	}

	var availableDay models.ParkingSpotAvailableDay
	if err := database.DB.
		Where("parking_spot_id = ? AND available_date = ? AND is_available = ?", input.SpotID, startDate, true).
		First(&availableDay).Error; err != nil {
		log.Printf("Parking spot %d is not available on %s: %v", input.SpotID, startDate, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "車位在指定日期不可用",
			"error":   "parking spot is not available on the specified date",
			"code":    "ERR_DATE_NOT_AVAILABLE",
		})
		return
	}

	rent := &models.Rent{
		MemberID:  currentMemberIDInt,
		SpotID:    input.SpotID,
		StartTime: startTime,
		EndTime:   endTime,
		Status:    "pending",
	}

	if err := services.RentParkingSpot(rent); err != nil {
		log.Printf("Failed to rent parking spot %d for member %d: %v", rent.SpotID, rent.MemberID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "租用車位失敗",
			"error":   err.Error(),
			"code":    "ERR_RENT_FAILED",
		})
		return
	}

	parkingSpot.Status = "pending"
	if err := database.DB.Save(&parkingSpot).Error; err != nil {
		log.Printf("Failed to update parking spot status for spot %d: %v", parkingSpot.SpotID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "更新車位狀態失敗",
			"error":   err.Error(),
			"code":    "ERR_UPDATE_STATUS",
		})
		return
	}

	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(rent, rent.RentID).Error; err != nil {
		log.Printf("Failed to preload rent data for rent ID %d: %v", rent.RentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "載入租用資料失敗",
			"error":   err.Error(),
			"code":    "ERR_PRELOAD_FAILED",
		})
		return
	}

	// 移除 FetchAvailableDays 調用，直接使用空切片
	availableDays := []models.ParkingSpotAvailableDay{}

	var parkingSpotRents []models.Rent
	if err := database.DB.Where("spot_id = ?", rent.SpotID).Find(&parkingSpotRents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", rent.SpotID, err)
		parkingSpotRents = []models.Rent{}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "車位租用成功",
		"data":    rent.ToResponse(availableDays, parkingSpotRents),
	})
}

// ReserveParkingSpot 處理車位預約請求
func ReserveParkingSpot(c *gin.Context) {
	var input RentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data for reservation: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的輸入資料",
			"error":   "請提供車位 ID、開始時間和結束時間",
			"code":    "ERR_INVALID_INPUT",
		})
		return
	}

	// 解析開始時間和結束時間
	startTime, err := parseTimeWithCST(input.StartTime)
	if err != nil {
		log.Printf("Failed to parse start_time %s: %v", input.StartTime, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的開始時間格式",
			"error":   err.Error(),
			"code":    "ERR_INVALID_TIME_FORMAT",
		})
		return
	}

	endTime, err := parseTimeWithCST(input.EndTime)
	if err != nil {
		log.Printf("Failed to parse end_time %s: %v", input.EndTime, err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的結束時間格式",
			"error":   err.Error(),
			"code":    "ERR_INVALID_TIME_FORMAT",
		})
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "member_id not found in token",
			"code":    "ERR_NO_MEMBER_ID",
		})
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid member_id type",
			"code":    "ERR_INVALID_MEMBER_ID",
		})
		return
	}

	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.FixedZone("CST", 8*60*60))
	if startTime.Before(today) {
		log.Printf("Reservation start time %s is before today %s", startTime.Format("2006-01-02T15:04:05"), today.Format("2006-01-02T15:04:05"))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "開始時間必須在今天或未來",
			"error":   "start_time must be today or in the future",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	if !endTime.After(startTime) {
		log.Printf("Reservation end time %s is not after start time %s", endTime.Format("2006-01-02T15:04:05"), startTime.Format("2006-01-02T15:04:05"))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "結束時間必須晚於開始時間",
			"error":   "end_time must be after start_time",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	// 移除 WiFi 驗證相關代碼
	var parkingSpot models.ParkingSpot
	if err := database.DB.Preload("Rents", "status IN (?)", []string{"pending", "reserved"}).First(&parkingSpot, input.SpotID).Error; err != nil {
		log.Printf("Failed to find parking spot %d: %v", input.SpotID, err)
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "車位不存在",
			"error":   "parking spot not found",
			"code":    "ERR_NOT_FOUND",
		})
		return
	}

	for _, rent := range parkingSpot.Rents {
		if (startTime.Before(rent.EndTime) || startTime.Equal(rent.EndTime)) &&
			(endTime.After(rent.StartTime) || endTime.Equal(rent.StartTime)) {
			log.Printf("Reservation time range overlaps with existing rent for spot %d: rent_id %d", input.SpotID, rent.RentID)
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "車位在指定時間範圍內不可用",
				"error":   "parking spot is not available for the specified time range",
				"code":    "ERR_TIME_OVERLAP",
			})
			return
		}
	}

	startDate := startTime.Format("2006-01-02")
	var availableDayCount int64
	if err := database.DB.Model(&models.ParkingSpotAvailableDay{}).
		Where("parking_spot_id = ? AND available_date = ? AND is_available = ?", input.SpotID, startDate, true).
		Count(&availableDayCount).Error; err != nil {
		log.Printf("Failed to check available days for spot %d: %v", input.SpotID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "檢查車位可用性失敗",
			"error":   err.Error(),
			"code":    "ERR_CHECK_AVAILABILITY",
		})
		return
	}

	if availableDayCount == 0 {
		log.Printf("Parking spot %d is not available on %s", input.SpotID, startDate)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "車位在指定日期不可用",
			"error":   "parking spot is not available on the specified date",
			"code":    "ERR_DATE_NOT_AVAILABLE",
		})
		return
	}

	reservation := &models.Rent{
		MemberID:  currentMemberIDInt,
		SpotID:    input.SpotID,
		StartTime: startTime,
		EndTime:   endTime,
		Status:    "reserved",
	}

	if err := services.ReserveParkingSpot(reservation); err != nil {
		log.Printf("Failed to reserve parking spot %d for member %d: %v", reservation.SpotID, reservation.MemberID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "預約車位失敗",
			"error":   err.Error(),
			"code":    "ERR_RESERVE_FAILED",
		})
		return
	}

	parkingSpot.Status = "reserved"
	if err := database.DB.Save(&parkingSpot).Error; err != nil {
		log.Printf("Failed to update parking spot status for spot %d: %v", parkingSpot.SpotID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "更新車位狀態失敗",
			"error":   err.Error(),
			"code":    "ERR_UPDATE_STATUS",
		})
		return
	}

	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(reservation, reservation.RentID).Error; err != nil {
		log.Printf("Failed to preload reservation data for rent ID %d: %v", reservation.RentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "載入預約資料失敗",
			"error":   err.Error(),
			"code":    "ERR_PRELOAD_FAILED",
		})
		return
	}

	// 移除 FetchAvailableDays 調用，直接使用空切片
	availableDays := []models.ParkingSpotAvailableDay{}

	var parkingSpotRents []models.Rent
	if err := database.DB.Where("spot_id = ?", reservation.SpotID).Find(&parkingSpotRents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", reservation.SpotID, err)
		parkingSpotRents = []models.Rent{}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "車位預約成功",
		"data":    reservation.ToResponse(availableDays, parkingSpotRents),
	})
}

// GetRentRecords 查詢租用紀錄資料檢查
func GetRentRecords(c *gin.Context) {
	currentMemberID, exists := c.Get("member_id")
	if !exists {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type")
		return
	}

	var rents []models.Rent
	if err := database.DB.Where("member_id = ?", currentMemberIDInt).Find(&rents).Error; err != nil {
		log.Printf("Failed to get rent records: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢租用紀錄失敗", err.Error())
		return
	}

	rentResponses := make([]models.SimpleRentResponse, len(rents))
	for i, rent := range rents {
		rentResponses[i] = rent.ToSimpleResponse()
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", rentResponses)
}

// GetMemberRentHistory 查詢特定會員的租賃歷史記錄
func GetMemberRentHistory(c *gin.Context) {
	requestedMemberIDStr := c.Param("id")
	requestedMemberID, err := strconv.Atoi(requestedMemberIDStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的會員 ID", err.Error())
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type")
		return
	}

	role, exists := c.Get("role")
	if !exists {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "role not found in token")
		return
	}
	roleStr, ok := role.(string)
	if !ok {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid role type")
		return
	}

	// 查詢會員是否存在
	var member models.Member
	if err := database.DB.First(&member, requestedMemberID).Error; err != nil {
		log.Printf("Failed to find member: member_id=%d, error=%v", requestedMemberID, err)
		ErrorResponse(c, http.StatusNotFound, "會員不存在", "member not found")
		return
	}

	var rents []models.Rent
	if roleStr == "shared_owner" {
		// shared_owner：查詢自己車位的租賃記錄
		if err := database.DB.
			Preload("Member").
			Preload("ParkingSpot").
			Where("spot_id IN (SELECT spot_id FROM parking_spots WHERE member_id = ?)", currentMemberIDInt).
			Find(&rents).Error; err != nil {
			log.Printf("Failed to get rent records for shared_owner: member_id=%d, error=%v", currentMemberIDInt, err)
			ErrorResponse(c, http.StatusInternalServerError, "查詢租用紀錄失敗", err.Error())
			return
		}
	} else {
		// renter 或 admin：查詢指定會員的租賃記錄
		if roleStr != "admin" && currentMemberIDInt != requestedMemberID {
			ErrorResponse(c, http.StatusForbidden, "無權限", "you can only view your own rent history")
			return
		}

		if err := database.DB.
			Preload("Member").
			Preload("ParkingSpot").
			Where("member_id = ?", requestedMemberID).
			Find(&rents).Error; err != nil {
			log.Printf("Failed to get rent records: member_id=%d, error=%v", requestedMemberID, err)
			ErrorResponse(c, http.StatusInternalServerError, "查詢租用紀錄失敗", err.Error())
			return
		}
	}

	// 轉換為回應格式
	rentResponses := make([]models.RentResponse, len(rents))
	for i, rent := range rents {
		// 移除 FetchAvailableDays 調用，直接使用空切片
		availableDays := []models.ParkingSpotAvailableDay{}

		var parkingSpotRents []models.Rent
		if err := database.DB.Where("spot_id = ?", rent.SpotID).Find(&parkingSpotRents).Error; err != nil {
			log.Printf("Failed to fetch rents: spot_id=%d, error=%v", rent.SpotID, err)
			parkingSpotRents = []models.Rent{}
		}

		rentResponses[i] = rent.ToResponse(availableDays, parkingSpotRents)
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", rentResponses)
}

// CancelRent 取消租用資料檢查
func CancelRent(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid rent ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的租用ID",
			"error":   err.Error(),
			"code":    "ERR_INVALID_ID",
		})
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "member_id not found in token",
			"code":    "ERR_NO_MEMBER_ID",
		})
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid member_id type",
			"code":    "ERR_INVALID_MEMBER_ID",
		})
		return
	}

	role, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "role not found in token",
			"code":    "ERR_NO_ROLE",
		})
		return
	}

	roleStr, ok := role.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid role type",
			"code":    "ERR_INVALID_ROLE",
		})
		return
	}

	var rent models.Rent
	if err := database.DB.Preload("ParkingSpot").First(&rent, id).Error; err != nil {
		log.Printf("Failed to find rent ID %d: %v", id, err)
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "租用記錄不存在",
			"error":   "rent record not found",
			"code":    "ERR_NOT_FOUND",
		})
		return
	}

	// 檢查權限：只有租賃的擁有者或 admin 能取消
	if roleStr != "admin" && rent.MemberID != currentMemberIDInt {
		log.Printf("Unauthorized attempt to cancel rent ID %d by member %d", id, currentMemberIDInt)
		c.JSON(http.StatusForbidden, gin.H{
			"status":  false,
			"message": "無權限",
			"error":   "you can only cancel your own rent",
			"code":    "ERR_INSUFFICIENT_PERMISSIONS",
		})
		return
	}

	if rent.ActualEndTime != nil {
		log.Printf("Attempted to cancel already settled rent ID %d", id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法取消",
			"error":   "租賃已結算",
			"code":    "ERR_ALREADY_SETTLED",
		})
		return
	}

	if rent.Status != "pending" && rent.Status != "reserved" {
		log.Printf("Cannot cancel rent ID %d with status %s", id, rent.Status)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法取消",
			"error":   "只能取消狀態為 pending 或 reserved 的記錄",
			"code":    "ERR_INVALID_STATUS",
		})
		return
	}

	rent.Status = "canceled"
	if err := database.DB.Save(&rent).Error; err != nil {
		log.Printf("Failed to cancel rent ID %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "取消租用失敗",
			"error":   err.Error(),
			"code":    "ERR_CANCEL_FAILED",
		})
		return
	}

	// 檢查是否有其他未結束的租賃或預約
	var activeRentCount int64
	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	if err := database.DB.Model(&models.Rent{}).
		Where("spot_id = ? AND status IN (?, ?) AND end_time >= ?", rent.SpotID, "pending", "reserved", now).
		Count(&activeRentCount).Error; err != nil {
		log.Printf("Failed to check active rents for spot %d: %v", rent.SpotID, err)
		activeRentCount = 0
	}

	// 檢查當天的可用性
	var isDayAvailable bool
	todayStr := now.Format("2006-01-02")
	var availableDayCount int64
	if err := database.DB.Model(&models.ParkingSpotAvailableDay{}).
		Where("parking_spot_id = ? AND available_date = ? AND is_available = ?", rent.SpotID, todayStr, true).
		Count(&availableDayCount).Error; err != nil {
		log.Printf("Failed to check available days for spot %d: %v", rent.SpotID, err)
	}
	isDayAvailable = availableDayCount > 0

	// 更新車位狀態
	if activeRentCount > 0 {
		rent.ParkingSpot.Status = "reserved"
	} else if isDayAvailable {
		rent.ParkingSpot.Status = "available"
	} else {
		rent.ParkingSpot.Status = "occupied"
	}

	if err := database.DB.Save(&rent.ParkingSpot).Error; err != nil {
		log.Printf("Failed to update parking spot status for spot %d: %v", rent.ParkingSpot.SpotID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "更新車位狀態失敗",
			"error":   err.Error(),
			"code":    "ERR_UPDATE_STATUS",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "取消租用成功",
	})
}

// LeaveAndPay 離開和付款資料檢查
func LeaveAndPay(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid rent ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的租賃 ID",
			"error":   "invalid rent ID: must be a number",
			"code":    "ERR_INVALID_ID",
		})
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "member_id not found in token",
			"code":    "ERR_NO_MEMBER_ID",
		})
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid member_id type",
			"code":    "ERR_INVALID_MEMBER_ID",
		})
		return
	}

	role, exists := c.Get("role")
	if !exists {
		log.Printf("Failed to get role from context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "role not found in token",
			"code":    "ERR_NO_ROLE",
		})
		return
	}

	roleStr, ok := role.(string)
	if !ok {
		log.Printf("Invalid role type in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid role type",
			"code":    "ERR_INVALID_ROLE",
		})
		return
	}

	var rent models.Rent
	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(&rent, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"status":  false,
				"message": "租賃記錄不存在",
				"error":   "record not found",
				"code":    "ERR_NOT_FOUND",
			})
			return
		}
		log.Printf("Failed to get rent: rent_id=%d, error=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to get rent record: database error",
			"code":    "ERR_DATABASE",
		})
		return
	}

	// 假設資料庫中的時間為 CST，設置時區
	cstZone := time.FixedZone("CST", 8*60*60)
	rent.StartTime = rent.StartTime.In(cstZone)
	rent.EndTime = rent.EndTime.In(cstZone)
	if rent.ActualEndTime != nil {
		*rent.ActualEndTime = rent.ActualEndTime.In(cstZone)
	}

	if rent.ParkingSpot.SpotID == 0 {
		log.Printf("ParkingSpot not found for rent ID %d, SpotID=%d", id, rent.SpotID)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "parking spot not found",
			"code":    "ERR_NOT_FOUND",
		})
		return
	}

	if roleStr != "admin" && rent.MemberID != currentMemberIDInt {
		log.Printf("Unauthorized attempt to settle rent ID %d by member %d", id, currentMemberIDInt)
		c.JSON(http.StatusForbidden, gin.H{
			"status":  false,
			"message": "無權限",
			"error":   "you can only settle your own rent",
			"code":    "ERR_INSUFFICIENT_PERMISSIONS",
		})
		return
	}

	if rent.ActualEndTime != nil {
		log.Printf("Attempted to settle already settled rent ID %d", id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法結算",
			"error":   "租賃已結束",
			"code":    "ERR_ALREADY_SETTLED",
		})
		return
	}

	if rent.ParkingSpot.PricePerHalfHour <= 0 || rent.ParkingSpot.DailyMaxPrice <= 0 {
		log.Printf("Invalid pricing for parking spot ID %d: PricePerHalfHour=%.2f, DailyMaxPrice=%.2f", rent.SpotID, rent.ParkingSpot.PricePerHalfHour, rent.ParkingSpot.DailyMaxPrice)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "invalid pricing data for parking spot",
			"code":    "ERR_INVALID_PRICING",
		})
		return
	}

	var input struct {
		ActualEndTime string `json:"actual_end_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的輸入資料",
			"error":   err.Error(),
			"code":    "ERR_INVALID_INPUT",
		})
		return
	}

	actualEndTime, err := parseTimeWithCST(input.ActualEndTime)
	if err != nil {
		log.Printf("Invalid actual_end_time format: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   err.Error(),
			"code":    "ERR_INVALID_TIME_FORMAT",
		})
		return
	}

	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	log.Printf("Received actual_end_time: %s, parsed as CST: %s, current CST time: %s, start_time: %s",
		input.ActualEndTime, actualEndTime.Format("2006-01-02T15:04:05"),
		now.Format("2006-01-02T15:04:05"), rent.StartTime.Format("2006-01-02T15:04:05"))

	if actualEndTime.Before(rent.StartTime) {
		log.Printf("Actual end time %v is before start time %v for rent ID %d", actualEndTime, rent.StartTime, id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   "actual_end_time cannot be earlier than start_time",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	// 放寬檢查，允許 actual_end_time 比 now 早 5 秒
	const timeTolerance = 5 * time.Second
	if actualEndTime.Before(now.Add(-timeTolerance)) {
		log.Printf("Actual end time %v is too early compared to current CST time %v (tolerance: %v) for rent ID %d", actualEndTime, now, timeTolerance, id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   "actual_end_time is too early compared to current time",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}
	if actualEndTime.After(now) {
		log.Printf("Actual end time %v is after current CST time %v for rent ID %d", actualEndTime, now, id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   "actual_end_time cannot be later than current time",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred: rent_id=%d, error=%v", id, r)
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "結算失敗",
				"error":   "unexpected error occurred",
				"code":    "ERR_PANIC",
			})
		}
	}()

	totalCost, err := services.CalculateRentCost(rent, rent.ParkingSpot, actualEndTime)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to calculate rent cost: rent_id=%d, error=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to calculate rent cost: " + err.Error(),
			"code":    "ERR_CALCULATION_FAILED",
		})
		return
	}

	log.Printf("Calculated cost for rent ID %d: total_cost=%.2f", id, totalCost)

	rent.ActualEndTime = &actualEndTime
	rent.TotalCost = totalCost
	rent.Status = "completed"
	if err := tx.Save(&rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update rent: rent_id=%d, error=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to update rent record: database error",
			"code":    "ERR_DATABASE",
		})
		return
	}

	newStatus, err := services.UpdateParkingSpotStatus(tx, rent.SpotID, now, cstZone)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", rent.SpotID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to update parking spot status: " + err.Error(),
			"code":    "ERR_UPDATE_STATUS",
		})
		return
	}
	rent.ParkingSpot.Status = newStatus
	if err := tx.Save(&rent.ParkingSpot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status in DB: spot_id=%d, error=%v", rent.ParkingSpot.SpotID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to update parking spot status: database error",
			"code":    "ERR_DATABASE",
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction: rent_id=%d, error=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to commit transaction: database error",
			"code":    "ERR_DATABASE",
		})
		return
	}

	// 移除 FetchAvailableDays 調用，直接使用空切片
	availableDays := []models.ParkingSpotAvailableDay{}

	var parkingSpotRents []models.Rent
	if err := database.DB.Where("spot_id = ?", rent.SpotID).Find(&parkingSpotRents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", rent.SpotID, err)
		parkingSpotRents = []models.Rent{}
	}

	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(&rent, id).Error; err != nil {
		log.Printf("Failed to reload rent data: rent_id=%d, error=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to reload rent data: database error",
			"code":    "ERR_DATABASE",
		})
		return
	}

	log.Printf("Successfully processed leave and pay: rent_id=%d, total_cost=%.2f", id, totalCost)

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "離開結算成功",
		"data":    rent.ToResponse(availableDays, parkingSpotRents),
	})
}

// GetRentByID 查詢特定租賃記錄
func GetRentByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid rent ID: id=%s, error=%v", idStr, err)
		ErrorResponse(c, http.StatusBadRequest, "無效的租用ID", err.Error())
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type")
		return
	}

	role, exists := c.Get("role")
	if !exists {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "role not found in token")
		return
	}
	roleStr, ok := role.(string)
	if !ok {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid role type")
		return
	}

	rent, availableDays, err := services.GetRentByID(id, currentMemberIDInt, roleStr)
	if err != nil {
		log.Printf("Failed to get rent: rent_id=%d, member_id=%d, error=%v", id, currentMemberIDInt, err)
		ErrorResponse(c, http.StatusNotFound, "租賃記錄不存在或無權訪問", err.Error())
		return
	}

	var parkingSpotRents []models.Rent
	if err := database.DB.Where("spot_id = ?", rent.SpotID).Find(&parkingSpotRents).Error; err != nil {
		log.Printf("Failed to fetch rents: spot_id=%d, error=%v", rent.SpotID, err)
		parkingSpotRents = []models.Rent{}
	}

	// 移除 FetchAvailableDays 調用，直接使用返回的 availableDays
	SuccessResponse(c, http.StatusOK, "查詢成功", rent.ToResponse(availableDays, parkingSpotRents))
}

// 處理預約超時 預約轉租賃
func ConfirmReservation(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid rent ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的租用ID",
			"error":   err.Error(),
			"code":    "ERR_INVALID_ID",
		})
		return
	}

	var rent models.Rent
	if err := database.DB.Preload("ParkingSpot").First(&rent, id).Error; err != nil {
		log.Printf("Failed to find rent ID %d: %v", id, err)
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "租用記錄不存在",
			"error":   "rent record not found",
			"code":    "ERR_NOT_FOUND",
		})
		return
	}

	if rent.Status != "reserved" {
		log.Printf("Rent ID %d is not a reservation, current status: %s", id, rent.Status)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法確認",
			"error":   "只能確認狀態為 reserved 的記錄",
			"code":    "ERR_INVALID_STATUS",
		})
		return
	}

	now := time.Now().In(time.FixedZone("CST", 8*60*60))
	if now.Before(rent.StartTime) {
		log.Printf("Cannot confirm reservation ID %d before start time", id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法確認",
			"error":   "預約尚未開始",
			"code":    "ERR_NOT_STARTED",
		})
		return
	}

	rent.Status = "pending"
	if err := database.DB.Save(&rent).Error; err != nil {
		log.Printf("Failed to confirm reservation ID %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "確認預約失敗",
			"error":   err.Error(),
			"code":    "ERR_CONFIRM_FAILED",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "預約確認成功，已轉為租賃",
	})
}

// GetCurrentlyRentedSpots 查詢目前正在租用中的車位
func GetCurrentlyRentedSpots(c *gin.Context) {
	// 直接從上下文獲取 member_id 和 role，無需檢查
	currentMemberIDInt := c.GetInt("member_id")
	roleStr := c.GetString("role")

	rents, err := services.GetCurrentlyRentedSpots(currentMemberIDInt, roleStr)
	if err != nil {
		log.Printf("Failed to get currently rented spots: error=%v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢失敗", err.Error())
		return
	}

	rentResponses := make([]models.RentResponse, len(rents))
	for i, rent := range rents {
		// 移除 FetchAvailableDays 調用，直接使用空切片
		availableDays := []models.ParkingSpotAvailableDay{}

		var parkingSpotRents []models.Rent
		if err := database.DB.Where("spot_id = ?", rent.SpotID).Find(&parkingSpotRents).Error; err != nil {
			log.Printf("Failed to fetch rents for spot %d: error=%v", rent.SpotID, err)
			parkingSpotRents = []models.Rent{}
		}

		rentResponses[i] = rent.ToResponse(availableDays, parkingSpotRents)
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", rentResponses)
}

// GetAllReservations 查詢所有 reserved 狀態的記錄
func GetAllReservations(c *gin.Context) {
	// 從上下文獲取 member_id
	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "member_id not found in token",
			"code":    "ERR_NO_MEMBER_ID",
		})
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid member_id type",
			"code":    "ERR_INVALID_MEMBER_ID",
		})
		return
	}

	// 從上下文獲取 role
	role, exists := c.Get("role")
	if !exists {
		log.Printf("Failed to get role from context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "role not found in token",
			"code":    "ERR_NO_ROLE",
		})
		return
	}

	roleStr, ok := role.(string)
	if !ok {
		log.Printf("Invalid role type in context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "invalid role type",
			"code":    "ERR_INVALID_ROLE",
		})
		return
	}

	// 調用服務層查詢預約記錄
	reservations, err := services.GetAllReservations(currentMemberIDInt, roleStr)
	if err != nil {
		if err.Error() == "insufficient role permissions: role="+roleStr {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  false,
				"message": "權限不足",
				"error":   "Insufficient role permissions",
				"code":    "ERR_INSUFFICIENT_PERMISSIONS",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "failed to fetch reservations: database error",
			"code":    "ERR_DATABASE",
		})
		return
	}

	// 準備回應資料
	var responseData []models.RentResponse
	for _, reservation := range reservations {
		// 由於不需要 availableDays 和 parkingSpotRents，傳入空值
		responseData = append(responseData, reservation.ToResponse([]models.ParkingSpotAvailableDay{}, []models.Rent{}))
	}

	// 成功回應
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    responseData,
	})
}
