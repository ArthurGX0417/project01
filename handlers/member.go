package handlers

import (
	"log"
	"net/http"
	"project01/database"
	"project01/models"
	"project01/services"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 電子郵件驗證 regex
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// 電話驗證字串 (例如：10 位數)
var phoneRegex = regexp.MustCompile(`^[0-9]{10}$`)

// 註冊會員資料檢查
func RegisterMember(c *gin.Context) {
	var member models.Member
	if err := c.ShouldBindJSON(&member); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入資料"})
		return
	}

	// 驗證電子郵件
	if member.Email == "" || !emailRegex.MatchString(member.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請提供有效的電子郵件地址"})
		return
	}

	// 驗證電話
	if member.Phone == "" || !phoneRegex.MatchString(member.Phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請提供有效的電話號碼（10位數字）"})
		return
	}

	// 驗證密碼（例如，最少 8 個字元，至少一個字母和一個數字）
	if len(member.Password) < 8 || !regexp.MustCompile(`[a-zA-Z]`).MatchString(member.Password) || !regexp.MustCompile(`[0-9]`).MatchString(member.Password) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密碼必須至少8個字符，包含字母和數字"})
		return
	}

	if member.PaymentMethod != "credit_card" && member.PaymentMethod != "e_wallet" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "payment_method 必須是 'credit_card' 或 'e_wallet'"})
		return
	}

	if member.PaymentInfo == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請提供 payment_info"})
		return
	}

	// 檢查現有的電子郵件或電話
	var existingMember models.Member
	if err := database.DB.Where("email = ?", member.Email).First(&existingMember).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "該電子郵件已被註冊"})
		return
	}
	if err := database.DB.Where("phone = ?", member.Phone).First(&existingMember).Error; err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "該電話號碼已被註冊"})
		return
	}

	if err := services.RegisterMember(&member); err != nil {
		log.Printf("Failed to register member with email %s and phone %s: %v", member.Email, member.Phone, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "會員註冊成功",
		"data":    member.ToResponse(),
	})
}

// 登入會員資料檢查
func LoginMember(c *gin.Context) {
	var loginData struct {
		Email    string `json:"email"`
		Phone    string `json:"phone"`
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&loginData); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入資料"})
		return
	}

	// 確保至少提供 email 或 phone
	if loginData.Email == "" && loginData.Phone == "" {
		log.Printf("No email or phone provided")
		c.JSON(http.StatusBadRequest, gin.H{"error": "必須提供電子郵件或電話"})
		return
	}

	// 驗證電子郵件（如有提供
	if loginData.Email != "" && !emailRegex.MatchString(loginData.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請提供有效的電子郵件地址"})
		return
	}

	// 驗證電話（如有提供
	if loginData.Phone != "" && !phoneRegex.MatchString(loginData.Phone) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "請提供有效的電話號碼（10位數字）"})
		return
	}

	// 驗證密碼
	if len(loginData.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密碼格式錯誤"})
		return
	}

	// 驗證登入憑證
	member, err := services.LoginMember(loginData.Email, loginData.Phone, loginData.Password)
	if err != nil {
		log.Printf("Login failed for email %s or phone %s: %v", loginData.Email, loginData.Phone, err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "登入失敗，檢查電子郵件、電話或密碼"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "登入成功",
		"data":    member.ToResponse(),
	})
}

// 根據會員資料檢查
func GetMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的會員ID"})
		return
	}

	member, err := services.GetMemberByID(id)
	if err != nil {
		log.Printf("Database error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "伺服器錯誤"})
		return
	}
	if member == nil {
		log.Printf("Member with ID %d not found", id)
		c.JSON(http.StatusNotFound, gin.H{"error": "會員不存在"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    member.ToResponse(),
	})
}

// 查詢所有會員資料檢查
func GetAllMembers(c *gin.Context) {
	members, err := services.GetAllMembers()
	if err != nil {
		log.Printf("Failed to get all members: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查詢所有會員失敗"})
		return
	}

	memberResponses := make([]models.MemberResponse, len(members))
	for i, member := range members {
		memberResponses[i] = member.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    memberResponses,
	})
}

// 根據ID更新會員資料檢查
func UpdateMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的會員ID"})
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

	if err := services.UpdateMember(id, updatedFields); err != nil {
		log.Printf("Failed to update member with ID %d and fields %v: %v", id, updatedFields, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	updatedMember, err := services.GetMemberByID(id)
	if err != nil {
		log.Printf("Failed to fetch updated member with ID %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "獲取更新後的會員資料失敗"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "更新成功",
		"data":    updatedMember.ToResponse(),
	})
}

// 刪除會員資料檢查
func DeleteMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的會員ID"})
		return
	}

	if err := services.DeleteMember(id); err != nil {
		if err.Error() == "會員不存在" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			log.Printf("Failed to delete member with ID %d: %v", id, err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "刪除會員失敗"})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "刪除成功",
	})
}

// GetMemberHistory 查詢特定會員的租賃歷史記錄
func GetMemberHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的會員ID"})
		return
	}

	rents, err := services.GetMemberRentHistory(id)
	if err != nil {
		log.Printf("Failed to get rent history: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查詢租賃歷史失敗"})
		return
	}

	// Fetch all available days for all parking spots in a single query
	spotIDs := make([]int, len(rents))
	for i, rent := range rents {
		spotIDs[i] = rent.SpotID
	}

	var availableDaysRecords []models.ParkingSpotAvailableDay
	availableDaysMap := make(map[int][]models.ParkingSpotAvailableDay)
	if len(spotIDs) > 0 {
		if err := database.DB.Where("spot_id IN ?", spotIDs).Find(&availableDaysRecords).Error; err != nil {
			log.Printf("Failed to fetch available days for spots: %v", err)
			availableDaysRecords = []models.ParkingSpotAvailableDay{}
		}
		for _, record := range availableDaysRecords {
			availableDaysMap[record.SpotID] = append(availableDaysMap[record.SpotID], record)
		}
	}

	rentResponses := make([]models.RentResponse, len(rents))
	for i, rent := range rents {
		availableDays := availableDaysMap[rent.SpotID]
		if availableDays == nil {
			availableDays = []models.ParkingSpotAvailableDay{}
		}
		rentResponses[i] = rent.ToResponse(availableDays)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    rentResponses,
	})
}
