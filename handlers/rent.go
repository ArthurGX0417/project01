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
	MemberID  int       `json:"member_id" binding:"required"`
	SpotID    int       `json:"spot_id" binding:"required"`
	StartTime time.Time `json:"start_time" binding:"required"`
	EndTime   time.Time `json:"end_time" binding:"required"`
}

// RentParkingSpot 租車位資料檢查
func RentParkingSpot(c *gin.Context) {
	// 綁定請求到 RentInput 結構體
	var input RentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的輸入資料",
			"error":   err.Error(),
		})
		return
	}

	// 從 token 中提取當前用戶的 member_id
	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		c.JSON(http.StatusUnauthorized, gin.H{
			"status":  false,
			"message": "未授權",
			"error":   "member_id not found in token",
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
		})
		return
	}

	// 檢查請求中的 member_id 是否與當前用戶一致
	if currentMemberIDInt != input.MemberID {
		log.Printf("Member %d attempted to rent parking spot for member %d", currentMemberIDInt, input.MemberID)
		c.JSON(http.StatusForbidden, gin.H{
			"status":  false,
			"message": "無權限",
			"error":   "you can only rent parking spots for yourself",
		})
		return
	}

	// 驗證 StartTime 是否在未來
	if input.StartTime.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "開始時間必須在未來",
		})
		return
	}

	// 檢查 WiFi 驗證
	wifiVerified, err := services.VerifyWifi(input.MemberID)
	if err != nil {
		log.Printf("Failed to verify WiFi for member %d: %v", input.MemberID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "WiFi 驗證失敗",
			"error":   err.Error(),
		})
		return
	}
	if !wifiVerified {
		c.JSON(http.StatusForbidden, gin.H{
			"status":  false,
			"message": "請通過 WiFi 驗證以使用服務",
		})
		return
	}

	// 將 RentInput 轉換為 models.Rent
	rent := &models.Rent{
		MemberID:  input.MemberID,
		SpotID:    input.SpotID,
		StartTime: input.StartTime,
		EndTime:   input.EndTime,
	}

	// 調用服務層方法
	if err := services.RentParkingSpot(rent); err != nil {
		log.Printf("Failed to rent parking spot %d for member %d: %v", rent.SpotID, rent.MemberID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "租用車位失敗",
			"error":   err.Error(),
		})
		return
	}

	// 預加載關聯數據，包括嵌套的 ParkingSpot.Member
	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(rent, rent.RentID).Error; err != nil {
		log.Printf("Failed to preload rent data for rent ID %d: %v", rent.RentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "載入租用資料失敗",
			"error":   err.Error(),
		})
		return
	}

	// 獲取車位的可用日期
	availableDays, err := services.FetchAvailableDays(rent.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", rent.SpotID, err)
		availableDays = []models.ParkingSpotAvailableDay{}
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "車位租用成功",
		"data":    rent.ToResponse(availableDays),
	})
}

// GetRentRecords 查詢租用紀錄資料檢查
func GetRentRecords(c *gin.Context) {
	rents, err := services.GetRentRecords()
	if err != nil {
		log.Printf("Failed to get rent records: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢租用紀錄失敗",
			"error":   err.Error(),
		})
		return
	}

	// 使用 SimpleRentResponse 減少回應大小
	rentResponses := make([]models.SimpleRentResponse, len(rents))
	for i, rent := range rents {
		rentResponses[i] = rent.ToSimpleResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    rentResponses,
	})
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

	if err := services.CancelRent(id); err != nil {
		log.Printf("Failed to cancel rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "取消租用失敗",
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
	// 解析租賃 ID
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

	// 查詢租賃記錄，預加載相關資料
	var rent models.Rent
	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").Preload("ParkingSpot.Rents").First(&rent, id).Error; err != nil {
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

	// 檢查是否已結算
	if rent.ActualEndTime != nil {
		log.Printf("Attempted to settle already settled rent ID %d", id)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無法結算",
			"error":   "租賃已結束",
		})
		return
	}

	// 解析請求中的 leave_time
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

	// 檢查離開時間是否早於開始時間
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
	durationHours := leaveTime.Sub(rent.StartTime).Hours()
	durationDays := durationHours / 24

	if rent.ParkingSpot.PricingType == "monthly" {
		// 按月計費，假設月費為 5000（可根據需求調整）
		const monthlyRate = 5000.0
		months := math.Ceil(durationDays / 30) // 每 30 天算一個月
		totalCost = months * monthlyRate
	} else {
		// 按小時計費
		halfHours := math.Ceil(durationHours * 2) // 每半小時計費一次
		totalCost = halfHours * float64(rent.ParkingSpot.PricePerHalfHour)
		// 考慮每日上限
		dailyMax := float64(rent.ParkingSpot.DailyMaxPrice)
		days := math.Ceil(durationDays)
		maxCost := dailyMax * days
		totalCost = math.Min(totalCost, maxCost)
	}

	// 更新租賃記錄
	rent.ActualEndTime = &leaveTime
	rent.TotalCost = totalCost

	// 保存更新
	if err := database.DB.Save(&rent).Error; err != nil {
		log.Printf("Failed to update rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to update rent record: database error",
		})
		return
	}

	// 獲取車位的可用日期
	availableDays, err := services.FetchAvailableDays(rent.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", rent.SpotID, err)
		availableDays = []models.ParkingSpotAvailableDay{}
	}

	// 重新加載更新後的租賃記錄
	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").Preload("ParkingSpot.Rents").First(&rent, id).Error; err != nil {
		log.Printf("Failed to reload rent data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "結算失敗",
			"error":   "failed to reload rent data: database error",
		})
		return
	}

	// 返回回應
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "離開結算成功",
		"data":    rent.ToResponse(availableDays),
	})
}

// GetRentByID 查詢特定租賃記錄
func GetRentByID(c *gin.Context) {
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

	rent, availableDays, err := services.GetRentByID(id)
	if err != nil {
		log.Printf("Failed to get rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢租賃記錄失敗",
			"error":   err.Error(),
		})
		return
	}
	if rent == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "租賃記錄不存在",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    rent.ToResponse(availableDays),
	})
}
