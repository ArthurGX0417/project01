package handlers

import (
	"fmt"
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
	AvailableDays    []AvailableDayInput `json:"available_days" binding:"omitempty,dive"`
}

func ShareParkingSpot(c *gin.Context) {
	var input ParkingSpotInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
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
		Status:           "idle", // 設置預設值，避免服務層處理
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
		availableDays[i] = models.ParkingSpotAvailableDay{
			AvailableDate: day.Date,
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
	location := c.Query("location")
	date := c.Query("date")

	// 接收所有三個返回值
	spots, availableDaysList, err := services.GetAvailableParkingSpots(location, date)
	if err != nil {
		log.Printf("Failed to get available parking spots: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢可用停車位失敗", err.Error())
		return
	}

	// 將 availableDaysList 與 spots 關聯起來
	spotResponses := make([]models.ParkingSpotResponse, len(spots))
	for i, spot := range spots {
		// 為每個停車位設置可用日期
		spotResponses[i] = spot.ToResponse(availableDaysList[i])
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", spotResponses)
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
