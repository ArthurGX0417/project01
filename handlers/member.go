package handlers

import (
	"errors"
	"log"
	"net/http"
	"project01/database"
	"project01/models"
	"project01/services"
	"project01/utils"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// 電子郵件驗證 regex
var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// 電話驗證字串 (例如：10 位數)
var phoneRegex = regexp.MustCompile(`^[0-9]{10}$`)

// 註冊會員資料檢查（保持不變）
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

	// 驗證密碼
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

	// 驗證並設置 role（不允許註冊為 admin）
	if member.Role != "shared_owner" && member.Role != "renter" {
		member.Role = "renter" // 預設為 renter
	}
	if member.Role == "admin" {
		ErrorResponse(c, http.StatusBadRequest, "不允許註冊為管理員", "admin role is not allowed for registration")
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

	if loginData.Email == "" && loginData.Phone == "" {
		log.Printf("No email or phone provided")
		ErrorResponse(c, http.StatusBadRequest, "必須提供電子郵件或電話", "email and phone are empty")
		return
	}

	if loginData.Email != "" && !emailRegex.MatchString(loginData.Email) {
		ErrorResponse(c, http.StatusBadRequest, "請提供有效的電子郵件地址", "invalid email format")
		return
	}

	if loginData.Phone != "" && !phoneRegex.MatchString(loginData.Phone) {
		ErrorResponse(c, http.StatusBadRequest, "請提供有效的電話號碼（10位數字）", "invalid phone format")
		return
	}

	if len(loginData.Password) < 8 || !regexp.MustCompile(`[a-zA-Z]`).MatchString(loginData.Password) || !regexp.MustCompile(`[0-9]`).MatchString(loginData.Password) {
		ErrorResponse(c, http.StatusBadRequest, "密碼必須至少8個字符，包含字母和數字", "invalid password format")
		return
	}

	member, err := services.LoginMember(loginData.Email, loginData.Phone, loginData.Password)
	if err != nil {
		log.Printf("Login failed for email %s or phone %s: %v", loginData.Email, loginData.Phone, err)
		if err.Error() == "member not found" {
			ErrorResponse(c, http.StatusUnauthorized, "電子郵件或電話不存在", err.Error())
		} else if err.Error() == "invalid password" {
			ErrorResponse(c, http.StatusUnauthorized, "密碼錯誤", err.Error())
		} else {
			ErrorResponse(c, http.StatusUnauthorized, "登入失敗，請稍後再試", err.Error())
		}
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"member_id": member.MemberID,
		"role":      member.Role, // 加入 role
		"exp":       time.Now().Add(time.Hour * 24).Unix(),
	})

	tokenString, err := token.SignedString(utils.JWTSecret)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "無法生成 token", err.Error())
		return
	}

	log.Printf("Member logged in successfully: email=%s, member_id=%d, role=%s", member.Email, member.MemberID, member.Role)

	SuccessResponse(c, http.StatusOK, "登入成功", gin.H{
		"token":  tokenString,
		"member": member.ToResponse(),
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

// GetMemberProfile 查看個人資料
func GetMemberProfile(c *gin.Context) {
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

	member, err := services.GetMemberProfileData(currentMemberIDInt)
	if err != nil {
		log.Printf("Failed to get member: %v", err)
		ErrorResponse(c, http.StatusInternalServerError, "查詢會員失敗", err.Error(), "ERR_INTERNAL_SERVER")
		return
	}
	if member == nil {
		ErrorResponse(c, http.StatusNotFound, "會員不存在", "member not found", "ERR_MEMBER_NOT_FOUND")
		return
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", member.ToResponse())
	log.Printf("Successfully retrieved profile for member %d", currentMemberIDInt)
}

// UpdateLicensePlate 更新車牌資訊
func UpdateLicensePlate(c *gin.Context) {
	type LicensePlateInput struct {
		LicensePlate string `json:"license_plate" binding:"required,max=20"`
	}

	var input LicensePlateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("Invalid input data: %v", err)
		ErrorResponse(c, http.StatusBadRequest, "無效的輸入資料", err.Error())
		return
	}

	// 從 JWT 獲取當前 member_id（由 AuthMiddleware 提供）
	currentMemberID, exists := c.Get("member_id")
	if !exists {
		log.Printf("Member ID not found in token")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "member_id not found in token")
		return
	}
	currentMemberIDInt, ok := currentMemberID.(int)
	if !ok {
		log.Printf("Invalid member_id type in context")
		ErrorResponse(c, http.StatusUnauthorized, "未授權", "invalid member_id type")
		return
	}

	// 呼叫服務層更新車牌
	if err := services.UpdateLicensePlate(currentMemberIDInt, input.LicensePlate); err != nil {
		log.Printf("Failed to update license plate: %v", err)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ErrorResponse(c, http.StatusNotFound, "會員不存在", err.Error())
		} else {
			ErrorResponse(c, http.StatusInternalServerError, "更新車牌失敗", err.Error())
		}
		return
	}

	// 成功回應
	SuccessResponse(c, http.StatusOK, "車牌更新成功", gin.H{
		"member_id":     currentMemberIDInt,
		"license_plate": input.LicensePlate,
	})
}
