package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"project01/models"
	"project01/services"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RentInput 定義用於綁定請求的輸入結構體
type RentInput struct {
	LicensePlate string `json:"license_plate" binding:"required"`
	ParkingLotID int    `json:"parking_lot_id" binding:"required,min=1"` // 新增，必填>0
	StartTime    string `json:"start_time" binding:"required"`
}

// LeaveInput 定義出場請求的輸入結構體
type LeaveInput struct {
	LicensePlate string `json:"license_plate" binding:"required"`
	EndTime      string `json:"end_time" binding:"required"`
}

// NotificationInput 定義通知請求的輸入結構體
type NotificationInput struct {
	RentID int `json:"rent_id" binding:"required"`
}

// parseTimeWithCST 解析時間字符串，確保儲存為 CST（+08:00）
func parseTimeWithCST(timeStr string) (time.Time, error) {
	log.Printf("Received time string: %s", timeStr)

	t, err := time.Parse(time.RFC3339, timeStr)
	if err == nil {
		cstZone := time.FixedZone("CST", 8*60*60)
		t = t.In(cstZone)
		log.Printf("Converted to CST for storage: %s", t.Format("2006-01-02T15:04:05+08:00"))
		return t, nil
	}

	cstZone := time.FixedZone("CST", 8*60*60)
	t, err = time.ParseInLocation("2006-01-02T15:04:05", timeStr, cstZone)
	if err == nil {
		log.Printf("Parsed time %s as CST: %s (assumed CST)", timeStr, t.Format("2006-01-02T15:04:05+08:00"))
		return t, nil
	}

	return time.Time{}, fmt.Errorf("time must be in 'YYYY-MM-DDThh:mm:ss' or RFC 3339 format")
}

// EnterParkingSpot 進場
func EnterParkingSpot(c *gin.Context) {
	var input RentInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	startTime, err := parseTimeWithCST(input.StartTime)
	if err != nil {
		log.Printf("Failed to parse start_time %s: %v", input.StartTime, err)
		ErrorResponse(c, http.StatusBadRequest, "無效的開始時間", err.Error())
		return
	}

	if err := services.EnterParkingSpot(input.LicensePlate, input.ParkingLotID, startTime); err != nil {
		log.Printf("Failed to enter parking spot: license_plate=%s, error=%v", input.LicensePlate, err)
		ErrorResponse(c, http.StatusInternalServerError, "進場失敗", err.Error())
		return
	}

	SuccessResponse(c, http.StatusOK, "進場成功", nil)
}

// LeaveParkingSpot 出場
// LeaveParkingSpot 出場（強制回傳 total_cost！）
func LeaveParkingSpot(c *gin.Context) {
	var input LeaveInput
	if err := c.ShouldBindJSON(&input); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	endTime, err := parseTimeWithCST(input.EndTime)
	if err != nil {
		ErrorResponse(c, http.StatusBadRequest, "無效的結束時間", err.Error())
		return
	}

	// 改這裡！收到 rent 物件
	rentRecord, err := services.LeaveParkingSpot(input.LicensePlate, endTime)
	if err != nil {
		log.Printf("Leave parking failed: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "出場失敗", err.Error())
		return
	}

	// 直接回傳完整資訊給前端！
	SuccessResponse(c, http.StatusOK, "出場成功，本次停車費用已計算", map[string]interface{}{
		"parking_record": rentRecord.ToResponse(),
		"total_cost":     rentRecord.TotalCost, // 前端最愛的欄位
		"duration_hours": endTime.Sub(rentRecord.StartTime).Hours(),
	})
}

// GetCurrentlyRentedSpots 查詢當前租用的車位
func GetCurrentlyRentedSpots(c *gin.Context) {
	licensePlate := c.GetString("license_plate") // 從 token 中獲取
	if licensePlate == "" {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "license_plate not found in token")
		return
	}

	rents, err := services.GetCurrentlyRentedSpots(licensePlate)
	if err != nil {
		log.Printf("Failed to get currently rented spots: license_plate=%s, error=%v", licensePlate, err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢失敗", err.Error())
		return
	}

	rentResponses := make([]models.RentResponse, len(rents))
	for i, rent := range rents {
		rentResponses[i] = rent.ToResponse()
	}
	SuccessResponse(c, http.StatusOK, "查詢成功", rentResponses)
}

// GetRentRecordsByLicensePlate 查詢租用紀錄
func GetRentRecordsByLicensePlate(c *gin.Context) {
	licensePlate := c.GetString("license_plate") // 從 token 中獲取
	if licensePlate == "" {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "license_plate not found in token")
		return
	}

	rents, err := services.GetRentRecordsByLicensePlate(licensePlate, licensePlate)
	if err != nil {
		log.Printf("Failed to get rent records: license_plate=%s, error=%v", licensePlate, err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ErrorResponse(c, http.StatusNotFound, "無租賃記錄", err.Error())
		} else {
			ErrorResponse(c, http.StatusForbidden, "無權限", err.Error())
		}
		return
	}

	rentResponses := make([]models.RentResponse, len(rents))
	for i, rent := range rents {
		rentResponses[i] = rent.ToResponse()
	}
	SuccessResponse(c, http.StatusOK, "查詢成功", rentResponses)
}

// GetTotalCostByLicensePlate 查詢總費用
func GetTotalCostByLicensePlate(c *gin.Context) {
	licensePlate := c.GetString("license_plate") // 從 token 中獲取
	if licensePlate == "" {
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "license_plate not found in token")
		return
	}

	totalCost, err := services.GetTotalCostByLicensePlate(licensePlate)
	if err != nil {
		log.Printf("Failed to get total cost: license_plate=%s, error=%v", licensePlate, err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢失敗", err.Error())
		return
	}
	SuccessResponse(c, http.StatusOK, "查詢成功", gin.H{"total_cost": totalCost})
}

// CheckParkingAvailability 查詢特定停車場總可用位子
func CheckParkingAvailability(c *gin.Context) {
	parkingLotIDStr := c.Param("id") // 從路徑取 :id
	parkingLotID := 0
	if parkingLotIDStr != "" {
		var err error
		parkingLotID, err = strconv.Atoi(parkingLotIDStr)
		if err != nil || parkingLotID <= 0 {
			ErrorResponse(c, http.StatusBadRequest, "無效的停車場ID", err.Error())
			return
		}
	}

	availableSpots, err := services.CheckParkingAvailability(parkingLotID)
	if err != nil {
		log.Printf("Failed to check parking availability: error=%v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢失敗", err.Error())
		return
	}
	SuccessResponse(c, http.StatusOK, "查詢成功", gin.H{"available_spots": availableSpots})
}
