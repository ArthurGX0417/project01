package handlers

import (
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
	SpotID    int       `json:"spot_id" binding:"required"`
	StartTime time.Time `json:"start_time" binding:"required"`
	EndTime   time.Time `json:"end_time" binding:"required"`
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
	log.Printf("Current UTC time: %s, StartTime: %s", now.Format(time.RFC3339), input.StartTime.Format(time.RFC3339))

	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if input.StartTime.Before(today) {
		log.Printf("Start time %s is before today %s", input.StartTime.Format(time.RFC3339), today.Format(time.RFC3339))
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "開始時間必須在今天或未來",
			"error":   "start_time must be today or in the future",
			"code":    "ERR_INVALID_TIME",
		})
		return
	}

	if !input.EndTime.After(input.StartTime) {
		log.Printf("End time %s is not after start time %s", input.EndTime.Format(time.RFC3339), input.StartTime.Format(time.RFC3339))
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
		// 檢查時間重疊（包括已結束的租賃）
		if (input.StartTime.Before(rent.EndTime) || input.StartTime.Equal(rent.EndTime)) &&
			(input.EndTime.After(rent.StartTime) || input.EndTime.Equal(rent.StartTime)) {
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
	startDate := input.StartTime.Format("2006-01-02")
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
		StartTime: input.StartTime,
		EndTime:   input.EndTime,
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
		})
		return
	}

	if rent.ActualEndTime != nil {
		log.Printf("Attempted to cancel already settled rent ID %d", id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法取消",
			"error":   "租賃已結算",
		})
		return
	}

	if err := services.CancelRent(id); err != nil {
		log.Printf("Failed to cancel rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "取消租用失敗",
			"error":   err.Error(),
		})
		return
	}

	// 檢查是否有其他未結束的租賃
	var activeRentCount int64
	if err := database.DB.Model(&models.Rent{}).
		Where("spot_id = ? AND actual_end_time IS NULL AND end_time >= ?", rent.SpotID, time.Now()).
		Count(&activeRentCount).Error; err != nil {
		log.Printf("Failed to check active rents for spot %d: %v", rent.SpotID, err)
		activeRentCount = 0
	}

	// 檢查當天的可用性
	var isDayAvailable bool
	todayStr := time.Now().Format("2006-01-02")
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
		LeaveTime string `json:"leave_time" binding:"required"`
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

	leaveTime, err := time.Parse(time.RFC3339, input.LeaveTime)
	if err != nil {
		log.Printf("Invalid leave_time format: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   "leave_time must be in ISO 8601 format (e.g., 2025-04-11T12:30:00Z)",
		})
		return
	}

	if leaveTime.Before(rent.StartTime) {
		log.Printf("Leave time %v is before start time %v for rent ID %d", leaveTime, rent.StartTime, id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的離開時間",
			"error":   "離開時間必須晚於租賃開始時間",
		})
		return
	}

	// 計算費用
	var totalCost float64
	durationMinutes := leaveTime.Sub(rent.StartTime).Minutes()
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

	rent.ActualEndTime = &leaveTime
	rent.TotalCost = totalCost

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
		Where("spot_id = ? AND actual_end_time IS NULL AND end_time >= ?", rent.SpotID, time.Now()).
		Count(&activeRentCount).Error; err != nil {
		log.Printf("Failed to check active rents for spot %d: %v", rent.SpotID, err)
		activeRentCount = 0
	}

	var isDayAvailable bool
	todayStr := time.Now().Format("2006-01-02")
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
		log.Printf("Invalid rent ID: %v", err)
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

	var rent models.Rent
	if err := database.DB.Where("rent_id = ? AND member_id = ?", id, currentMemberIDInt).First(&rent).Error; err != nil {
		log.Printf("Failed to get rent: %v", err)
		ErrorResponse(c, http.StatusNotFound, "租賃記錄不存在", "rent record not found")
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

	SuccessResponse(c, http.StatusOK, "查詢成功", rent.ToResponse(availableDays, parkingSpotRents))
}
