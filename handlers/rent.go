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

	if err := services.EnterParkingSpot(input.LicensePlate, startTime); err != nil {
		log.Printf("Failed to enter parking spot: license_plate=%s, error=%v", input.LicensePlate, err)
		ErrorResponse(c, http.StatusInternalServerError, "進場失敗", err.Error())
		return
	}

	SuccessResponse(c, http.StatusOK, "進場成功", nil)
}

// LeaveParkingSpot 出場
func LeaveParkingSpot(c *gin.Context) {
	var input LeaveInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	endTime, err := parseTimeWithCST(input.EndTime)
	if err != nil {
		log.Printf("Failed to parse end_time %s: %v", input.EndTime, err)
		ErrorResponse(c, http.StatusBadRequest, "無效的結束時間", err.Error())
		return
	}

	if err := services.LeaveParkingSpot(input.LicensePlate, endTime); err != nil {
		log.Printf("Failed to leave parking spot: license_plate=%s, error=%v", input.LicensePlate, err)
		ErrorResponse(c, http.StatusInternalServerError, "出場失敗", err.Error())
		return
	}

	SuccessResponse(c, http.StatusOK, "出場成功", nil)
}

// GenerateParkingNotification 生成停車通知
func GenerateParkingNotification(c *gin.Context) {
	var input NotificationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	// 假設需要從服務層實現通知邏輯（目前未定義）
	// 這裡暫時返回一個模擬回應，需在 services/rent.go 中添加對應函數
	notification := fmt.Sprintf("Parking notification for rent ID %d", input.RentID)
	SuccessResponse(c, http.StatusOK, "通知生成成功", gin.H{"notification": notification})
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

// GetRentByID 查詢特定租賃記錄
func GetRentByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid rent ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的租賃ID", err.Error())
		return
	}

	rent, err := services.GetRentByID(id)
	if err != nil {
		log.Printf("Failed to get rent by ID %d: %v", id, err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢失敗", err.Error())
		return
	}
	if rent == nil {
		ErrorResponse(c, http.StatusNotFound, "租賃記錄不存在", "rent not found")
		return
	}

	// 權限檢查：確保當前用戶的 license_plate 與 rent 一致
	licensePlate := c.GetString("license_plate")
	if licensePlate != rent.LicensePlate {
		ErrorResponse(c, http.StatusForbidden, "無權限", "unauthorized access to rent record")
		return
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", rent.ToResponse())
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

// CheckParkingAvailability 檢查停車場可用性
func CheckParkingAvailability(c *gin.Context) {
	availableSpots, err := services.CheckParkingAvailability()
	if err != nil {
		log.Printf("Failed to check parking availability: error=%v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢失敗", err.Error())
		return
	}
	SuccessResponse(c, http.StatusOK, "查詢成功", gin.H{"available_spots": availableSpots})
}
