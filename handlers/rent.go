package handlers

import (
	"fmt"
	"log"
	"math"
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

// parseTimeWithUTC 解析時間字符串並假設為 UTC
func parseTimeWithUTC(timeStr string) (time.Time, error) {
	// 嘗試解析不帶時區的格式
	t, err := time.Parse("2006-01-02T15:04:05", timeStr)
	if err == nil {
		// 假設默認時區為 UTC
		return t.UTC(), nil
	}

	// 如果不帶時區的格式解析失敗，嘗試 RFC 3339 格式
	t, err = time.Parse(time.RFC3339, timeStr)
	if err == nil {
		// 已經包含時區，直接轉為 UTC
		return t.UTC(), nil
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

	// 解析開始時間和結束時間
	startTime, err := parseTimeWithUTC(input.StartTime)
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

	endTime, err := parseTimeWithUTC(input.EndTime)
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

	now := time.Now().UTC()
	log.Printf("Current UTC time: %s, StartTime: %s", now.Format(time.RFC3339), startTime.Format(time.RFC3339))

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if startTime.Before(today) {
		log.Printf("Start time %s is before today %s", startTime.Format(time.RFC3339), today.Format(time.RFC3339))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "開始時間必須在今天或未來",
			"error":   "start_time must be today or in the future",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	if !endTime.After(startTime) {
		log.Printf("End time %s is not after start time %s", endTime.Format(time.RFC3339), startTime.Format(time.RFC3339))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "結束時間必須晚於開始時間",
			"error":   "end_time must be after start_time",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	wifiVerified, err := services.VerifyWifi(currentMemberIDInt)
	if err != nil {
		log.Printf("Failed to verify WiFi for member %d: %v", currentMemberIDInt, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "WiFi 驗證失敗",
			"error":   err.Error(),
			"code":    "ERR_WIFI_VERIFICATION",
		})
		return
	}
	if !wifiVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  false,
			"message": "請通過 WiFi 驗證以使用服務",
			"error":   "WiFi verification required",
			"code":    "ERR_WIFI_NOT_VERIFIED",
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
			log.Printf("Parking spot %d has an active rent: rent_id %d, end_time %s", input.SpotID, rent.RentID, rent.EndTime.Format(time.RFC3339))
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

	availableDays, err := services.FetchAvailableDays(rent.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", rent.SpotID, err)
		availableDays = []models.ParkingSpotAvailableDay{}
	}

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
	startTime, err := parseTimeWithUTC(input.StartTime)
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

	endTime, err := parseTimeWithUTC(input.EndTime)
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

	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if startTime.Before(today) {
		log.Printf("Reservation start time %s is before today %s", startTime.Format(time.RFC3339), today.Format(time.RFC3339))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "開始時間必須在今天或未來",
			"error":   "start_time must be today or in the future",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	if !endTime.After(startTime) {
		log.Printf("Reservation end time %s is not after start time %s", endTime.Format(time.RFC3339), startTime.Format(time.RFC3339))
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

	availableDays, err := services.FetchAvailableDays(reservation.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", reservation.SpotID, err)
		availableDays = []models.ParkingSpotAvailableDay{}
	}

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

// 以下函數保持不變
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
	if err := database.DB.Model(&models.Rent{}).
		Where("spot_id = ? AND status IN (?, ?) AND end_time >= ?", rent.SpotID, "pending", "reserved", time.Now().UTC()).
		Count(&activeRentCount).Error; err != nil {
		log.Printf("Failed to check active rents for spot %d: %v", rent.SpotID, err)
		activeRentCount = 0
	}

	// 檢查當天的可用性
	var isDayAvailable bool
	todayStr := time.Now().UTC().Format("2006-01-02")
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
			})
			return
		}
		log.Printf("Failed to get rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to get rent record: database error",
		})
		return
	}

	if rent.ActualEndTime != nil {
		log.Printf("Attempted to settle already settled rent ID %d", id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法結算",
			"error":   "租賃已結束",
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
		})
		return
	}

	actualEndTime, err := parseTimeWithUTC(input.ActualEndTime)
	if err != nil {
		log.Printf("Invalid actual_end_time format: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   "actual_end_time must be in 'YYYY-MM-DDThh:mm:ss' or RFC 3339 format",
		})
		return
	}

	if actualEndTime.Before(rent.StartTime) {
		log.Printf("Actual end time %v is before start time %v for rent ID %d", actualEndTime, rent.StartTime, id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   "離開時間必須晚於租賃開始時間",
		})
		return
	}

	// 計算費用
	var totalCost float64
	durationMinutes := actualEndTime.Sub(rent.StartTime).Minutes()
	durationDays := durationMinutes / (24 * 60)

	// 檢查是否在 5 分鐘內，若是則免費
	if durationMinutes <= 5 {
		totalCost = 0
	} else {
		if rent.ParkingSpot.PricingType == "monthly" {
			months := math.Ceil(durationDays / 30)
			totalCost = months * rent.ParkingSpot.MonthlyPrice
		} else { // hourly
			halfHours := math.Floor(durationMinutes / 30)
			remainingMinutes := durationMinutes - (halfHours * 30)
			if remainingMinutes > 5 { // 超過 5 分鐘才計入下一個半小時
				halfHours++
			}
			totalCost = halfHours * rent.ParkingSpot.PricePerHalfHour
			dailyMax := rent.ParkingSpot.DailyMaxPrice
			days := math.Ceil(durationDays)
			maxCost := dailyMax * days
			totalCost = math.Min(totalCost, maxCost)
		}
	}

	rent.ActualEndTime = &actualEndTime
	rent.TotalCost = totalCost
	rent.Status = "completed" // 當租賃結算完成時

	if err := database.DB.Save(&rent).Error; err != nil {
		log.Printf("Failed to update rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to update rent record: database error",
		})
		return
	}

	var activeRentCount int64
	if err := database.DB.Model(&models.Rent{}).
		Where("spot_id = ? AND actual_end_time IS NULL AND end_time >= ?", rent.SpotID, time.Now().UTC()).
		Count(&activeRentCount).Error; err != nil {
		log.Printf("Failed to check active rents for spot %d: %v", rent.SpotID, err)
		activeRentCount = 0
	}

	var isDayAvailable bool
	todayStr := time.Now().UTC().Format("2006-01-02")
	var availableDayCount int64
	if err := database.DB.Model(&models.ParkingSpotAvailableDay{}).
		Where("parking_spot_id = ? AND available_date = ? AND is_available = ?", rent.SpotID, todayStr, true).
		Count(&availableDayCount).Error; err != nil {
		log.Printf("Failed to check available days for spot %d: %v", rent.SpotID, err)
	}
	isDayAvailable = availableDayCount > 0

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
			"message": "結算失敗",
			"error":   "failed to update parking spot status: database error",
		})
		return
	}

	availableDays, err := services.FetchAvailableDays(rent.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", rent.SpotID, err)
		availableDays = []models.ParkingSpotAvailableDay{}
	}

	var parkingSpotRents []models.Rent
	if err := database.DB.Where("spot_id = ?", rent.SpotID).Find(&parkingSpotRents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", rent.SpotID, err)
		parkingSpotRents = []models.Rent{}
	}

	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(&rent, id).Error; err != nil {
		log.Printf("Failed to reload rent data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to reload rent data: database error",
		})
		return
	}

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

	SuccessResponse(c, http.StatusOK, "查詢成功", rent.ToResponse(availableDays, parkingSpotRents))
}

// 查詢所有 reserved 狀態的記錄
func GetReservations(c *gin.Context) {
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

	var reservations []models.Rent
	if err := database.DB.Where("member_id = ? AND status = ?", currentMemberIDInt, "reserved").Find(&reservations).Error; err != nil {
		log.Printf("Failed to get reservations: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢預約記錄失敗",
			"error":   err.Error(),
			"code":    "ERR_FETCH_RESERVATIONS",
		})
		return
	}

	rentResponses := make([]models.SimpleRentResponse, len(reservations))
	for i, reservation := range reservations {
		rentResponses[i] = reservation.ToSimpleResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    rentResponses,
	})
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

	now := time.Now().UTC()
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
