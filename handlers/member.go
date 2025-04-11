package handlers

import (
	"log"
	"net/http"
	"os"
	"project01/database"
	"project01/models"
	"project01/services"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// 電子郵件驗證 regex
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// 電話驗證字串 (例如：10 位數)
var phoneRegex = regexp.MustCompile(`^[0-9]{10}$`)

// 定義並初始化 jwtSecret
var jwtSecret []byte

func init() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}
	jwtSecret = []byte(secret)
	log.Printf("Loaded JWT_SECRET in handlers: %s...", secret[:4])
}

// 註冊會員資料檢查
func RegisterMember(c *gin.Context) {
	var member models.Member
	if err := c.ShouldBindJSON(&member); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	// 驗證電子郵件
	if member.Email == "" || !emailRegex.MatchString(member.Email) {
		ErrorResponse(c, http.StatusBadRequest, "請提供有效的電子郵件地址", "invalid email format")
		return
	}

	// 驗證電話
	if member.Phone == "" || !phoneRegex.MatchString(member.Phone) {
		ErrorResponse(c, http.StatusBadRequest, "請提供有效的電話號碼（10位數字）", "invalid phone format")
		return
	}

	// 驗證密碼（例如，最少 8 個字元，至少一個字母和一個數字）
	if len(member.Password) < 8 || !regexp.MustCompile(`[a-zA-Z]`).MatchString(member.Password) || !regexp.MustCompile(`[0-9]`).MatchString(member.Password) {
		ErrorResponse(c, http.StatusBadRequest, "密碼必須至少8個字符，包含字母和數字", "invalid password format")
		return
	}

	if member.PaymentMethod != "credit_card" && member.PaymentMethod != "e_wallet" {
		ErrorResponse(c, http.StatusBadRequest, "payment_method 必須是 'credit_card' 或 'e_wallet'", "invalid payment_method")
		return
	}

	if member.PaymentInfo == "" {
		ErrorResponse(c, http.StatusBadRequest, "請提供 payment_info", "payment_info is required")
		return
	}

	// 檢查現有的電子郵件或電話
	var existingMember models.Member
	if err := database.DB.Where("email = ?", member.Email).First(&existingMember).Error; err == nil {
		ErrorResponse(c, http.StatusBadRequest, "該電子郵件已被註冊", "email already in use")
		return
	}
	if err := database.DB.Where("phone = ?", member.Phone).First(&existingMember).Error; err == nil {
		ErrorResponse(c, http.StatusBadRequest, "該電話號碼已被註冊", "phone already in use")
		return
	}

	if err := services.RegisterMember(&member); err != nil {
		log.Printf("Failed to register member with email %s and phone %s: %v", member.Email, member.Phone, err)
		ErrorResponse(c, http.StatusInternalServerError, "會員註冊失敗", err.Error())
		return
	}

	SuccessResponse(c, http.StatusOK, "會員註冊成功", member.ToResponse())
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
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	// 確保至少提供 email 或 phone
	if loginData.Email == "" && loginData.Phone == "" {
		log.Printf("No email or phone provided")
		ErrorResponse(c, http.StatusBadRequest, "必須提供電子郵件或電話", "email and phone are empty")
		return
	}

	// 驗證電子郵件
	if loginData.Email != "" && !emailRegex.MatchString(loginData.Email) {
		ErrorResponse(c, http.StatusBadRequest, "請提供有效的電子郵件地址", "invalid email format")
		return
	}

	// 驗證電話
	if loginData.Phone != "" && !phoneRegex.MatchString(loginData.Phone) {
		ErrorResponse(c, http.StatusBadRequest, "請提供有效的電話號碼（10位數字）", "invalid phone format")
		return
	}

	// 驗證密碼
	if len(loginData.Password) < 8 {
		ErrorResponse(c, http.StatusBadRequest, "密碼格式錯誤", "password must be at least 8 characters")
		return
	}

	// 驗證登入憑證
	member, err := services.LoginMember(loginData.Email, loginData.Phone, loginData.Password)
	if err != nil {
		log.Printf("Login failed for email %s or phone %s: %v", loginData.Email, loginData.Phone, err)
		ErrorResponse(c, http.StatusUnauthorized, "登入失敗，檢查電子郵件、電話或密碼", err.Error())
		return
	}

	// 生成 JWT
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"member_id": member.MemberID,
		"exp":       time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "無法生成 token", err.Error())
		return
	}

	// 記錄生成的 token（僅記錄前 10 個字符，避免安全風險）
	log.Printf("Generated JWT token: %s...", tokenString[:10])

	log.Printf("Member logged in successfully: email=%s", member.Email)
	SuccessResponse(c, http.StatusOK, "登入成功", gin.H{
		"member": member.ToResponse(),
		"token":  tokenString,
	})
}

// 根據會員資料檢查
func GetMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的會員ID", err.Error())
		return
	}

	member, err := services.GetMemberByID(id)
	if err != nil {
		log.Printf("Database error: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "伺服器錯誤", err.Error())
		return
	}
	if member == nil {
		log.Printf("Member with ID %d not found", id)
		ErrorResponse(c, http.StatusNotFound, "會員不存在", "member not found")
		return
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", member.ToResponseWithSpots(member.Spots))
}

// 查詢所有會員資料檢查
func GetAllMembers(c *gin.Context) {
	members, err := services.GetAllMembers()
	if err != nil {
		log.Printf("Failed to get all members: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢所有會員失敗", err.Error())
		return
	}

	log.Printf("Successfully retrieved %d members", len(members))

	memberResponses := make([]models.MemberResponse, len(members))
	for i, member := range members {
		memberResponses[i] = member.ToResponseWithSpots(member.Spots)
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", memberResponses)
}

// 根據ID更新會員資料檢查
func UpdateMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的會員ID", err.Error())
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

	if err := services.UpdateMember(id, updatedFields); err != nil {
		log.Printf("Failed to update member with ID %d and fields %v: %v", id, updatedFields, err)
		ErrorResponse(c, http.StatusInternalServerError, "更新會員失敗", err.Error())
		return
	}

	updatedMember, err := services.GetMemberByID(id)
	if err != nil {
		log.Printf("Failed to fetch updated member with ID %d: %v", id, err)
		ErrorResponse(c, http.StatusInternalServerError, "獲取更新後的會員資料失敗", err.Error())
		return
	}

	SuccessResponse(c, http.StatusOK, "更新成功", updatedMember.ToResponseWithSpots(updatedMember.Spots))
}

// 刪除會員資料檢查
func DeleteMember(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的會員ID", err.Error())
		return
	}

	if err := services.DeleteMember(id); err != nil {
		if err.Error() == "member with ID "+idStr+" not found" {
			ErrorResponse(c, http.StatusNotFound, "會員不存在", err.Error())
		} else {
			log.Printf("Failed to delete member with ID %d: %v", id, err)
			ErrorResponse(c, http.StatusInternalServerError, "刪除會員失敗", err.Error())
		}
		return
	}

	SuccessResponse(c, http.StatusOK, "刪除成功", nil)
}

// GetMemberHistory 查詢特定會員的租賃歷史記錄
func GetMemberHistory(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		log.Printf("Invalid member ID: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的會員ID", err.Error())
		return
	}

	rents, err := services.GetMemberRentHistory(id)
	if err != nil {
		log.Printf("Failed to get rent history: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢租賃歷史失敗", err.Error())
		return
	}

	// 一次查詢即可取得所有停車位的所有可用天數
	spotIDs := make([]int, len(rents))
	for i, rent := range rents {
		spotIDs[i] = rent.SpotID
	}

	var availableDaysRecords []models.ParkingSpotAvailableDay
	availableDaysMap := make(map[int][]models.ParkingSpotAvailableDay)
	if len(spotIDs) > 0 {
		if err := database.DB.Where("parking_spot_id IN ?", spotIDs).Find(&availableDaysRecords).Error; err != nil {
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

	SuccessResponse(c, http.StatusOK, "查詢成功", rentResponses)
}
