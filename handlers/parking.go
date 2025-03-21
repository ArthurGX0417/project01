// handlers/parking.go
package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"project01/database"
	"project01/models"
	"project01/services"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 中間結構用於綁定客戶端輸入
type ParkingSpotInput struct {
	MemberID         int      `json:"member_id"`
	ParkingType      string   `json:"parking_type"`
	FloorLevel       string   `json:"floor_level"`
	Location         string   `json:"location"`
	PricingType      string   `json:"pricing_type"`
	PricePerHalfHour float64  `json:"price_per_half_hour"`
	DailyMaxPrice    float64  `json:"daily_max_price"`
	Longitude        float64  `json:"longitude"`
	Latitude         float64  `json:"latitude"`
	AvailableDays    []string `json:"available_days"`
}

func ShareParkingSpot(c *gin.Context) {
	var input ParkingSpotInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入資料"})
		return
	}

	// 將輸入數據轉換為 ParkingSpot
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
	}

	// 序列化 available_days 為 JSON 字符串
	if len(input.AvailableDays) > 0 {
		daysJSON, err := json.Marshal(input.AvailableDays)
		if err != nil {
			log.Printf("Failed to marshal available_days: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "處理可用日期失敗"})
			return
		}
		spot.AvailableDays = string(daysJSON)
	} else {
		// 如果未設置，預設為全週可用
		defaultDays := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
		daysJSON, err := json.Marshal(defaultDays)
		if err != nil {
			log.Printf("Failed to marshal default available_days: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "處理預設可用日期失敗"})
			return
		}
		spot.AvailableDays = string(daysJSON)
	}

	if err := services.ShareParkingSpot(spot); err != nil {
		log.Printf("Failed to share parking spot: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存車位失敗"})
		return
	}

	// 重新查詢以加載關聯數據
	refreshedSpot, err := services.GetParkingSpotByID(spot.SpotID)
	if err != nil {
		log.Printf("Failed to refresh parking spot: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "刷新車位資料失敗"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "車位共享成功",
		"data":    refreshedSpot.ToResponse(),
	})
}

// 查詢可用停車位資料檢查
func GetAvailableParkingSpots(c *gin.Context) {
	var parkingSpots []models.ParkingSpot
	if err := database.DB.Where("status = ?", "idle").Preload("Member").Find(&parkingSpots).Error; err != nil {
		log.Printf("Failed to get available parking spots: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查詢可用車位失敗"})
		return
	}

	parkingSpotResponses := make([]models.ParkingSpotResponse, len(parkingSpots))
	for i, spot := range parkingSpots {
		parkingSpotResponses[i] = spot.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    parkingSpotResponses,
	})
}

// 查詢特定車位資料檢查
func GetParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的車位ID"})
		return
	}

	// 使用 services 層的 GetParkingSpotByID 來加載數據
	spot, err := services.GetParkingSpotByID(id)
	if err != nil {
		log.Printf("Failed to get parking spot: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查詢車位失敗"})
		return
	}
	if spot == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "車位不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    spot.ToResponse(),
	})
}

// UpdateParkingSpot 更新車位信息
func UpdateParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的車位ID"})
		return
	}

	var updatedFields map[string]interface{}
	if err := c.ShouldBindJSON(&updatedFields); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入資料"})
		return
	}

	if len(updatedFields) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "未提供任何更新字段"})
		return
	}

	if err := services.UpdateParkingSpot(id, updatedFields); err != nil {
		log.Printf("Failed to update parking spot with ID %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updatedSpot, err := services.GetParkingSpotByID(id)
	if err != nil {
		log.Printf("Failed to fetch updated parking spot with ID %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "獲取更新後的車位資料失敗"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "車位更新成功",
		"data":    updatedSpot.ToResponse(),
	})
}
