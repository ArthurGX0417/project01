package handlers

import (
	"log"
	"net/http"
	"project01/models"
	"project01/services"
	"strconv"

	"github.com/gin-gonic/gin"
)

// 註冊會員資料檢查
func RegisterMember(c *gin.Context) {
	var member models.Member
	if err := c.ShouldBindJSON(&member); err != nil {
		log.Printf("Invalid input data: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "無效的輸入資料"})
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

	if err := services.RegisterMember(&member); err != nil {
		log.Printf("Failed to register member: %v", err)
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

	// 驗證登入憑證
	member, err := services.LoginMember(loginData.Email, loginData.Phone, loginData.Password)
	if err != nil {
		log.Printf("Login failed: %v", err)
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

	rentResponses := make([]models.RentResponse, len(rents))
	for i, rent := range rents {
		rentResponses[i] = rent.ToResponse()
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "查詢成功",
		"data":    rentResponses,
	})
}
