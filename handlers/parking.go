package handlers

import (
	"fmt"
	"log"
	"net/http"
	"project01/database"
	"project01/models"
	"project01/services"
	"strconv"
	"strings"
	"time"

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
	MonthlyPrice     float64             `json:"monthly_price" binding:"gte=0"`
	Longitude        float64             `json:"longitude" binding:"required,gte=-180,lte=180"`
	Latitude         float64             `json:"latitude" binding:"required,gte=-90,lte=90"`
	AvailableDays    []AvailableDayInput `json:"available_days" binding:"omitempty,dive"`
}

func ShareParkingSpot(c *gin.Context) {
	var input ParkingSpotInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type")
		return
	}

	if currentMemberIDInt != input.MemberID {
		log.Printf("Member %d attempted to share parking spot for member %d", currentMemberIDInt, input.MemberID)
		ErrorResponse(c, http.StatusForbidden, "無權限", "you can only share parking spots for yourself")
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
		MonthlyPrice:     input.MonthlyPrice,
		Longitude:        input.Longitude,
		Latitude:         input.Latitude,
		Status:           "available",
	}

	availableDays := make([]models.ParkingSpotAvailableDay, len(input.AvailableDays))
	for i, day := range input.AvailableDays {
		parsedDate, err := time.Parse("2006-01-02", day.Date)
		if err != nil {
			log.Printf("Invalid date format for available_days: %v", err)
			ErrorResponse(c, http.StatusBadRequest, "無效的日期格式", "date must be in YYYY-MM-DD format")
			return
		}
		availableDays[i] = models.ParkingSpotAvailableDay{
			AvailableDate: parsedDate,
			IsAvailable:   day.IsAvailable,
		}
	}

	if err := services.ShareParkingSpot(spot, availableDays); err != nil {
		log.Printf("Failed to share parking spot for member %d: %v", input.MemberID, err)
		ErrorResponse(c, http.StatusInternalServerError, "儲存車位失敗", err.Error())
		return
	}

	refreshedSpot, availableDaysFetched, err := services.GetParkingSpotByID(spot.SpotID)
	if err != nil {
		log.Printf("Failed to refresh parking spot with ID %d: %v", spot.SpotID, err)
		ErrorResponse(c, http.StatusInternalServerError, "刷新車位資料失敗", err.Error())
		return
	}

	var rents []models.Rent
	if err := database.DB.Where("spot_id = ?", refreshedSpot.SpotID).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", refreshedSpot.SpotID, err)
		rents = []models.Rent{}
	}

	SuccessResponse(c, http.StatusOK, "車位共享成功", refreshedSpot.ToResponse(availableDaysFetched, rents))
}

func GetAvailableParkingSpots(c *gin.Context) {
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
	radius := 0.0 // 預設值為 0，表示使用服務層的預設值（3 公里）
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

	// 調用服務層函數，傳遞經緯度和 radius
	parkingSpots, availableDaysList, err := services.GetAvailableParkingSpots(dateStr, latitude, longitude, radius)
	if err != nil {
		log.Printf("Failed to get parking spots: %v", err)
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
				"error":   fmt.Sprintf("failed to get parking spots: %v", err),
			})
		}
		return
	}

	log.Printf("Queried %d parking spots for date %s:", len(parkingSpots), dateStr)
	for _, spot := range parkingSpots {
		log.Printf("Spot %d: Status=%s, AvailableDays=%v", spot.SpotID, spot.Status, spot.AvailableDays)
	}

	var availableSpots []models.ParkingSpot
	unavailableSpots := []int{}
	for i, spot := range parkingSpots {
		// 確保每個車位都有可用日期記錄
		spot.AvailableDays = availableDaysList[i]
		hasMatchingDate := false
		for _, day := range spot.AvailableDays {
			if day.AvailableDate.Format("2006-01-02") == dateStr && day.IsAvailable {
				hasMatchingDate = true
				break
			}
		}
		if hasMatchingDate {
			availableSpots = append(availableSpots, spot)
			log.Printf("Spot %d included in available spots", spot.SpotID)
		} else {
			unavailableSpots = append(unavailableSpots, spot.SpotID)
			log.Printf("Spot %d excluded: Not available on date %s", spot.SpotID, dateStr)
		}
	}

	log.Printf("Found %d parking spots, %d available after filtering for date %s", len(parkingSpots), len(availableSpots), dateStr)

	if len(availableSpots) == 0 {
		message := fmt.Sprintf("所選條件（日期：%s，經緯度：%s, %s）目前沒有符合的車位！請調整篩選條件。", dateStr, latitudeStr, longitudeStr)
		if len(unavailableSpots) > 0 {
			message += fmt.Sprintf(" 以下車位未設定可用日期：%v", unavailableSpots)
		}
		c.JSON(http.StatusOK, gin.H{
			"status":  false,
			"message": message,
			"data":    []models.ParkingSpot{},
		})
		return
	}

	availableSpotsResponse := make([]models.ParkingSpotResponse, len(availableSpots))
	for i, spot := range availableSpots {
		availableSpotsResponse[i] = spot.ToResponse(spot.AvailableDays, spot.Rents)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  true,
		"message": "查詢成功",
		"data":    availableSpotsResponse,
	})
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

	// 查詢該停車位的租賃記錄
	var rents []models.Rent
	if err := database.DB.Where("spot_id = ?", spot.SpotID).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", spot.SpotID, err)
		rents = []models.Rent{}
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", spot.ToResponse(availableDays, rents))
}

func UpdateParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的車位ID", err.Error())
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type")
		return
	}

	spot, _, err := services.GetParkingSpotByID(id)
	if err != nil {
		log.Printf("Failed to get parking spot: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢車位失敗", err.Error())
		return
	}
	if spot == nil {
		ErrorResponse(c, http.StatusNotFound, "車位不存在", "parking spot not found")
		return
	}

	if spot.MemberID != currentMemberIDInt {
		log.Printf("Member %d attempted to update parking spot %d owned by member %d", currentMemberIDInt, id, spot.MemberID)
		ErrorResponse(c, http.StatusForbidden, "無權限", "you are not the owner of this parking spot")
		return
	}

	member, err := services.GetMemberByID(currentMemberIDInt)
	if err != nil {
		log.Printf("Failed to get member: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢會員失敗", err.Error())
		return
	}
	if member == nil {
		ErrorResponse(c, http.StatusNotFound, "會員不存在", "member not found")
		return
	}
	if member.Role != "shared_owner" {
		log.Printf("Member %d with role %s attempted to update parking spot %d", currentMemberIDInt, member.Role, id)
		ErrorResponse(c, http.StatusForbidden, "無權限", "only shared_owner can update parking spots")
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

	// 查詢該停車位的租賃記錄
	var rents []models.Rent
	if err := database.DB.Where("spot_id = ?", updatedSpot.SpotID).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", updatedSpot.SpotID, err)
		rents = []models.Rent{}
	}

	SuccessResponse(c, http.StatusOK, "車位更新成功", updatedSpot.ToResponse(availableDays, rents))
}

// GetParkingSpotIncome 查看指定車位的收入（僅限 shared_owner 或 admin）
func GetParkingSpotIncome(c *gin.Context) {
	log.Printf("Received request for GetParkingSpotIncome with spot_id: %s", c.Param("id"))

	// 獲取車位 ID
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的車位 ID", "parking spot ID must be a number", "ERR_INVALID_SPOT_ID")
		return
	}

	// 獲取查詢參數 start_date 和 end_date
	startDateStr := c.Query("start_date")
	endDateStr := c.Query("end_date")
	log.Printf("Query parameters - start_date: %s, end_date: %s", startDateStr, endDateStr)

	// 驗證日期參數是否存在
	if startDateStr == "" || endDateStr == "" {
		log.Printf("Missing date range: start_date or end_date is empty")
		ErrorResponse(c, http.StatusBadRequest, "缺少日期範圍", "start_date and end_date are required", "ERR_MISSING_DATE_RANGE")
		return
	}

	// 解析日期
	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		log.Printf("Invalid start_date format: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的開始日期格式", "start_date must be in YYYY-MM-DD format", "ERR_INVALID_START_DATE")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		log.Printf("Invalid end_date format: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的結束日期格式", "end_date must be in YYYY-MM-DD format", "ERR_INVALID_END_DATE")
		return
	}

	// 確保開始日期不晚於結束日期
	if startDate.After(endDate) {
		log.Printf("Invalid date range: start_date %s is after end_date %s", startDateStr, endDateStr)
		ErrorResponse(c, http.StatusBadRequest, "日期範圍無效", "start_date cannot be later than end_date", "ERR_INVALID_DATE_RANGE")
		return
	}

	// 將 endDate 調整到當天的 23:59:59，以便包含整天的記錄
	endDate = time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 0, time.UTC)
	log.Printf("Adjusted date range - start_date: %s, end_date: %s", startDate.Format(time.RFC3339), endDate.Format(time.RFC3339))

	// 獲取當前會員 ID 和角色
	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Member ID not found in token")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token", "ERR_NO_MEMBER_ID")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type: %v", currentMemberID)
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type", "ERR_INVALID_MEMBER_ID_TYPE")
		return
	}

	currentRole, exists := c.Get("role")
	if !exists {
		log.Printf("Role not found in token")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "role not found in token", "ERR_NO_ROLE")
		return
	}

	currentRoleStr, ok := currentRole.(string)
	if !ok {
		log.Printf("Invalid role type: %v", currentRole)
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid role type", "ERR_INVALID_ROLE_TYPE")
		return
	}
	log.Printf("Authenticated member - member_id: %d, role: %s", currentMemberIDInt, currentRoleStr)

	// 調用 services 層計算收入，傳遞角色
	totalIncome, spot, err := services.GetParkingSpotIncome(id, startDate, endDate, currentMemberIDInt, currentRoleStr)
	if err != nil {
		log.Printf("Failed to get parking spot income: %v", err)
		if strings.Contains(err.Error(), "parking spot not found") {
			ErrorResponse(c, http.StatusNotFound, "車位不存在", "parking spot not found", "ERR_SPOT_NOT_FOUND")
		} else if strings.Contains(err.Error(), "permission denied") {
			ErrorResponse(c, http.StatusForbidden, "無權限", "you can only view income of your own parking spot", "ERR_INSUFFICIENT_PERMISSIONS")
		} else if strings.Contains(err.Error(), "invalid role") {
			ErrorResponse(c, http.StatusForbidden, "無權限", "invalid role", "ERR_INSUFFICIENT_PERMISSIONS")
		} else {
			ErrorResponse(c, http.StatusInternalServerError, "查詢車位收入失敗", err.Error(), "ERR_INTERNAL_SERVER")
		}
		return
	}

	// 返回收入數據
	response := gin.H{
		"spot_id":      spot.SpotID,
		"location":     spot.Location,
		"start_date":   startDateStr,
		"end_date":     endDateStr,
		"total_income": totalIncome,
	}

	log.Printf("Sending response: %v", response)
	SuccessResponse(c, http.StatusOK, "查詢車位收入成功", response)
}

func DeleteParkingSpot(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid parking spot ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的車位ID", err.Error(), "ERR_INVALID_SPOT_ID")
		return
	}

	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Failed to get member_id from context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token", "ERR_NO_MEMBER_ID")
		return
	}

	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type", "ERR_INVALID_MEMBER_ID_TYPE")
		return
	}

	currentRole, exists := c.Get("role")
	if !exists {
		log.Printf("Role not found in token")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "role not found in token", "ERR_NO_ROLE")
		return
	}

	currentRoleStr, ok := currentRole.(string)
	if !ok {
		log.Printf("Invalid role type: %v", currentRole)
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid role type", "ERR_INVALID_ROLE_TYPE")
		return
	}

	// 調用 services 層刪除車位
	err = services.DeleteParkingSpot(id, currentMemberIDInt, currentRoleStr)
	if err != nil {
		log.Printf("Failed to delete parking spot: %v", err)
		if strings.Contains(err.Error(), "parking spot not found") {
			ErrorResponse(c, http.StatusNotFound, "車位不存在", "parking spot not found", "ERR_SPOT_NOT_FOUND")
		} else if strings.Contains(err.Error(), "permission denied") {
			ErrorResponse(c, http.StatusForbidden, "無權限", "you can only delete your own parking spot", "ERR_PERMISSION_DENIED")
		} else {
			ErrorResponse(c, http.StatusInternalServerError, "刪除車位失敗", err.Error(), "ERR_INTERNAL_SERVER")
		}
		return
	}

	SuccessResponse(c, http.StatusOK, "車位刪除成功", nil)
}
