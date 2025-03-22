package handlers

import (
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

// RentParkingSpot 租車位資料檢查
func RentParkingSpot(c *gin.Context) {
	var rent models.Rent
	if err := c.ShouldBindJSON(&rent); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入資料"})
		return
	}

	// 驗證 StartTime 是否在未來
	if rent.StartTime.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "開始時間必須在未來"})
		return
	}

	wifiVerified, err := services.VerifyWifi(rent.MemberID)
	if err != nil {
		log.Printf("Failed to verify WiFi for member %d: %v", rent.MemberID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "WiFi 驗證失敗"})
		return
	}
	if !wifiVerified {
		c.JSON(http.StatusForbidden, gin.H{"error": "請通過 WiFi 驗證以使用服務"})
		return
	}

	if err := services.RentParkingSpot(&rent); err != nil {
		log.Printf("Failed to rent parking spot %d for member %d: %v", rent.SpotID, rent.MemberID, err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "租用車位失敗",
			"details": err.Error(),
		})
		return
	}

	// 預加載關聯數據，包括嵌套的 ParkingSpot.Member
	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(&rent, rent.RentID).Error; err != nil {
		log.Printf("Failed to preload rent data for rent ID %d: %v", rent.RentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "載入租用資料失敗"})
		return
	}

	// Fetch available days for the parking spot
	availableDays, err := services.FetchAvailableDays(rent.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", rent.SpotID, err)
		availableDays = []models.ParkingSpotAvailableDay{}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "車位租用成功",
		"data":    rent.ToResponse(availableDays),
	})
}

// GetRentRecords 查詢租用紀錄資料檢查
func GetRentRecords(c *gin.Context) {
	rents, err := services.GetRentRecords()
	if err != nil {
		log.Printf("Failed to get rent records: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查詢租用紀錄失敗"})
		return
	}

	// Use SimpleRentResponse to reduce response size
	rentResponses := make([]models.SimpleRentResponse, len(rents))
	for i, rent := range rents {
		rentResponses[i] = rent.ToSimpleResponse()
	}

	c.JSON(http.StatusOK, gin.H{
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的租用ID"})
		return
	}

	if err := services.CancelRent(id); err != nil {
		log.Printf("Failed to cancel rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "取消租用失敗"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "取消租用成功",
	})
}

// LeaveAndPay 離開和付款資料檢查
func LeaveAndPay(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid rent ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的租用ID"})
		return
	}

	var rent models.Rent
	if err := database.DB.Preload("Member").Preload("ParkingSpot").First(&rent, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "租用記錄不存在"})
			return
		}
		log.Printf("Failed to get rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "結算失敗"})
		return
	}

	// 檢查是否已結算
	if rent.ActualEndTime != nil {
		log.Printf("Attempted to settle already settled rent ID %d", id)
		c.JSON(http.StatusBadRequest, gin.H{"error": "租用已結算，無法再次結算"})
		return
	}

	// 解析請求中的 actual_end_time
	var input struct {
		ActualEndTime string `json:"actual_end_time" binding:"required"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入數據"})
		return
	}

	actualEndTime, err := time.Parse(time.RFC3339, input.ActualEndTime)
	if err != nil {
		log.Printf("Invalid actual_end_time format: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "實際離開時間格式錯誤"})
		return
	}

	// 調用 services.LeaveAndPay 計算費用並更新
	totalCost, err := services.LeaveAndPay(id, actualEndTime)
	if err != nil {
		log.Printf("Failed to process leave and pay: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "結算失敗",
			"details": err.Error(),
		})
		return
	}

	// 重新加載更新後的租用記錄
	if err := database.DB.Preload("Member").Preload("ParkingSpot").Preload("ParkingSpot.Member").First(&rent, id).Error; err != nil {
		log.Printf("Failed to reload rent data: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "載入租用資料失敗"})
		return
	}

	// Fetch available days for the parking spot
	availableDays, err := services.FetchAvailableDays(rent.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", rent.SpotID, err)
		availableDays = []models.ParkingSpotAvailableDay{}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":         "結算成功",
		"actual_end_time": rent.ActualEndTime,
		"total_cost":      totalCost,
		"data":            rent.ToResponse(availableDays),
	})
}

// GetRentByID 查詢特定租賃記錄
func GetRentByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid rent ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的租用ID"})
		return
	}

	rent, availableDays, err := services.GetRentByID(id)
	if err != nil {
		log.Printf("Failed to get rent: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查詢租賃記錄失敗"})
		return
	}
	if rent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "租賃記錄不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    rent.ToResponse(availableDays),
	})
}
