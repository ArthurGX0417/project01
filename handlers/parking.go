package handlers

import (
	"fmt"
	"log"
	"net/http"
	"project01/models"
	"project01/services"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func GetAvailableParkingLots(c *gin.Context) {
	// 獲取查詢參數
	dateStr := c.Query("date")
	latitudeStr := c.Query("latitude")
	longitudeStr := c.Query("longitude")
	radiusStr := c.Query("radius")

	log.Printf("Received request with date: %s, latitude: %s, longitude: %s, radius: %s", dateStr, latitudeStr, longitudeStr, radiusStr)

	// 驗證 date 參數
	if dateStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "請提供 date 參數（格式為 YYYY-MM-DD）",
		})
		return
	}

	// 驗證 latitude 和 longitude 參數
	if latitudeStr == "" || longitudeStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   "請提供 latitude 和 longitude 參數",
		})
		return
	}

	latitude, err := strconv.ParseFloat(latitudeStr, 64)
	if err != nil || latitude < -90 || latitude > 90 {
		log.Printf("Invalid latitude: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   fmt.Sprintf("無效的緯度，應為 -90 到 90 之間的數字: %v", err),
		})
		return
	}

	longitude, err := strconv.ParseFloat(longitudeStr, 64)
	if err != nil || longitude < -180 || longitude > 180 {
		log.Printf("Invalid longitude: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "查詢失敗",
			"error":   fmt.Sprintf("無效的經度，應為 -180 到 180 之間的數字: %v", err),
		})
		return
	}

	// 解析 radius 參數
	radius := 0.0
	if radiusStr != "" {
		radius, err = strconv.ParseFloat(radiusStr, 64)
		if err != nil || radius < 0 {
			log.Printf("Invalid radius: %v", err)
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "查詢失敗",
				"error":   fmt.Sprintf("無效的 radius，應為正數: %v", err),
			})
			return
		}
	}

	// 調用服務層函數
	parkingLots, err := services.GetAvailableParkingLots(dateStr, latitude, longitude, radius)
	if err != nil {
		log.Printf("Failed to get parking lots: %v", err)
		if strings.Contains(err.Error(), "invalid date format") {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "查詢失敗",
				"error":   "無效的日期格式，應為 YYYY-MM-DD",
			})
		} else if strings.Contains(err.Error(), "date must be today or in the future") {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "查詢失敗",
				"error":   "日期必須為今天或未來的日期",
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"status":  false,
				"message": "查詢失敗",
				"error":   fmt.Sprintf("failed to get parking lots: %v", err),
			})
		}
		return
	}

	log.Printf("Found %d parking lots available for date %s", len(parkingLots), dateStr)

	if len(parkingLots) == 0 {
		message := fmt.Sprintf("所選條件（日期：%s，經緯度：%s, %s）目前沒有符合的停車場！請調整篩選條件。", dateStr, latitudeStr, longitudeStr)
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": message,
			"data":    []models.ParkingSpot{},
		})
		return
	}

	// 轉換為回應格式
	availableLotsResponse := make([]models.ParkingSpotResponse, len(parkingLots))
	for i, lot := range parkingLots {
		availableLotsResponse[i] = lot.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    availableLotsResponse,
	})
}

func GetParkingLot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking lot ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{
			"status":  false,
			"message": "無效的停車場ID",
			"error":   err.Error(),
		})
		return
	}

	lot, err := services.GetParkingLotByID(id)
	if err != nil {
		log.Printf("Failed to get parking lot: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"status":  false,
			"message": "查詢停車場失敗",
			"error":   err.Error(),
		})
		return
	}
	if lot == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"status":  false,
			"message": "停車場不存在",
			"error":   "parking lot not found",
		})
		return
	}

	// 回應成功
	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    lot.ToResponse(),
	})
}
