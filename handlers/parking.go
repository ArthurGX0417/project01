package handlers

import (
	"log"
	"net/http"
	"project01/models"
	"project01/services"
	"strconv"

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
	AvailableDays    []AvailableDayInput `json:"available_days" binding:"required,dive"`
}

func ShareParkingSpot(c *gin.Context) {
	var input ParkingSpotInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入資料"})
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
	}

	availableDays := make([]models.ParkingSpotAvailableDay, len(input.AvailableDays))
	for i, day := range input.AvailableDays {
		availableDays[i] = models.ParkingSpotAvailableDay{
			AvailableDate: day.Date,
			IsAvailable:   day.IsAvailable,
		}
	}

	if err := services.ShareParkingSpot(spot, availableDays); err != nil {
		log.Printf("Failed to share parking spot for member %d: %v", input.MemberID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "儲存車位失敗"})
		return
	}

	refreshedSpot, availableDaysFetched, err := services.GetParkingSpotByID(spot.SpotID)
	if err != nil {
		log.Printf("Failed to refresh parking spot with ID %d: %v", spot.SpotID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "刷新車位資料失敗"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "車位共享成功",
		"data":    refreshedSpot.ToResponse(availableDaysFetched),
	})
}

func GetAvailableParkingSpots(c *gin.Context) {
	spots, availableDaysList, err := services.GetAvailableParkingSpots()
	if err != nil {
		log.Printf("Failed to get available parking spots: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查詢可用車位失敗"})
		return
	}

	parkingSpotResponses := make([]models.ParkingSpotResponse, len(spots))
	for i, spot := range spots {
		parkingSpotResponses[i] = spot.ToResponse(availableDaysList[i])
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    parkingSpotResponses,
	})
}

func GetParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的車位ID"})
		return
	}

	spot, availableDays, err := services.GetParkingSpotByID(id)
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
		"data":    spot.ToResponse(availableDays),
	})
}

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

	updatedSpot, availableDays, err := services.GetParkingSpotByID(id)
	if err != nil {
		log.Printf("Failed to fetch updated parking spot with ID %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "獲取更新後的車位資料失敗"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "車位更新成功",
		"data":    updatedSpot.ToResponse(availableDays),
	})
}
