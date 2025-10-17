package services

import (
	"errors"
	"fmt"
	"log"
	"project01/database"
	"project01/models"
	"project01/utils"
	"regexp"

	"gorm.io/gorm"
)

// RegisterMember 註冊會員
func RegisterMember(member *models.Member) error {
	// 檢查是否有重複的 email 或 phone
	var existingMember models.Member
	if err := database.DB.Where("email = ?", member.Email).First(&existingMember).Error; err == nil {
		return fmt.Errorf("email %s is already in use", member.Email)
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("Failed to check for duplicate email: %v", err)
		return fmt.Errorf("failed to check for duplicate email: %w", err)
	}

	if err := database.DB.Where("phone = ?", member.Phone).First(&existingMember).Error; err == nil {
		return fmt.Errorf("phone %s is already in use", member.Phone)
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("Failed to check for duplicate phone: %v", err)
		return fmt.Errorf("failed to check for duplicate phone: %w", err)
	}

	// 驗證密碼（至少 8 個字元，包含字母和數字）
	if len(member.Password) < 8 ||
		!regexp.MustCompile(`[a-zA-Z]`).MatchString(member.Password) ||
		!regexp.MustCompile(`[0-9]`).MatchString(member.Password) {
		return fmt.Errorf("password must be at least 8 characters and include both letters and numbers")
	}

	// 驗證 role
	if member.Role != "renter" && member.Role != "admin" {
		return fmt.Errorf("invalid role: must be 'renter' or 'admin'")
	}

	// 哈希密碼
	hashedPassword, err := utils.HashPassword(member.Password)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		return fmt.Errorf("failed to hash password: %w", err)
	}
	member.Password = hashedPassword

	// 加密 payment_info
	if member.PaymentInfo != "" {
		encryptedPaymentInfo, err := utils.EncryptPaymentInfo(member.PaymentInfo)
		if err != nil {
			log.Printf("Failed to encrypt payment_info: %v", err)
			return fmt.Errorf("failed to encrypt payment_info: %w", err)
		}
		member.PaymentInfo = encryptedPaymentInfo
	}

	// 使用 GORM 插入數據
	if err := database.DB.Create(member).Error; err != nil {
		log.Printf("Failed to register member: %v", err)
		return fmt.Errorf("failed to register member: %w", err)
	}

	log.Printf("Successfully registered member with ID %d", member.MemberID)
	return nil
}

// LoginMember 登入會員
func LoginMember(email, phone, password string) (*models.Member, error) {
	var member models.Member
	var err error

	// 根據 email 或 phone 查詢
	if email != "" {
		err = database.DB.Where("email = ?", email).First(&member).Error
	} else if phone != "" {
		err = database.DB.Where("phone = ?", phone).First(&member).Error
	} else {
		return nil, fmt.Errorf("no email or phone provided")
	}

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with email %s or phone %s not found", email, phone)
			return nil, fmt.Errorf("member not found")
		}
		log.Printf("Failed to login member: %v", err)
		return nil, fmt.Errorf("failed to login member: %w", err)
	}

	// 驗證密碼
	if !utils.CheckPasswordHash(password, member.Password) {
		log.Printf("Invalid password for email %s or phone %s", email, phone)
		return nil, fmt.Errorf("invalid password")
	}

	log.Printf("Member with ID %d logged in successfully", member.MemberID)
	return &member, nil
}

// GetMemberByID 根據ID查詢會員
func GetMemberByID(id int) (*models.Member, error) {
	var member models.Member
	if err := database.DB.First(&member, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with ID %d not found", id)
			return nil, nil
		}
		log.Printf("Failed to get member by ID %d: %v", id, err)
		return nil, fmt.Errorf("failed to get member by ID %d: %w", id, err)
	}

	// 解密 payment_info
	if member.PaymentInfo != "" {
		decryptedPaymentInfo, err := utils.DecryptPaymentInfo(member.PaymentInfo)
		if err != nil {
			log.Printf("Failed to decrypt payment_info for member %d: %v", id, err)
			member.PaymentInfo = ""
		} else {
			member.PaymentInfo = decryptedPaymentInfo
		}
	}

	log.Printf("Successfully retrieved member with ID %d", id)
	return &member, nil
}

// GetMemberProfileData 查詢會員的個人資料（僅基本資訊）
func GetMemberProfileData(memberID int) (*models.Member, error) {
	var member models.Member
	if err := database.DB.First(&member, memberID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with ID %d not found", memberID)
			return nil, nil
		}
		log.Printf("Failed to get member by ID %d: %v", memberID, err)
		return nil, fmt.Errorf("failed to get member by ID %d: %w", memberID, err)
	}

	// 解密 payment_info
	if member.PaymentInfo != "" {
		decryptedPaymentInfo, err := utils.DecryptPaymentInfo(member.PaymentInfo)
		if err != nil {
			log.Printf("Failed to decrypt payment_info for member %d: %v", memberID, err)
			member.PaymentInfo = ""
		} else {
			member.PaymentInfo = decryptedPaymentInfo
		}
	}

	log.Printf("Successfully retrieved member profile with ID %d", memberID)
	return &member, nil
}

// GetAllMembers 查詢所有會員
func GetAllMembers() ([]models.Member, error) {
	var members []models.Member
	if err := database.DB.Find(&members).Error; err != nil {
		log.Printf("Failed to query all members: %v", err)
		return nil, fmt.Errorf("failed to query all members: %w", err)
	}

	for i := range members {
		if members[i].PaymentInfo != "" {
			decryptedPaymentInfo, err := utils.DecryptPaymentInfo(members[i].PaymentInfo)
			if err != nil {
				log.Printf("Failed to decrypt payment_info for member %d: %v", members[i].MemberID, err)
			} else {
				members[i].PaymentInfo = decryptedPaymentInfo
			}
		}
	}

	log.Printf("Successfully retrieved %d members", len(members))
	return members, nil
}

// UpdateMember 更新某ID會員資訊
func UpdateMember(id int, updatedFields map[string]interface{}) error {
	var member models.Member
	// 檢查會員是否存在
	if err := database.DB.First(&member, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with ID %d not found", id)
			return fmt.Errorf("member with ID %d not found", id)
		}
		log.Printf("Failed to find member: %v", err)
		return fmt.Errorf("failed to find member with ID %d: %w", id, err)
	}

	// 映射 JSON 字段名到資料庫列名
	mappedFields := make(map[string]interface{})
	for key, value := range updatedFields {
		switch key {
		case "member_id":
			// 防止更新主鍵
			return fmt.Errorf("cannot update member_id")
		case "phone":
			phoneStr, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid phone type: must be a string")
			}
			// 檢查是否有重複的 phone
			var existingMember models.Member
			if err := database.DB.Where("phone = ? AND member_id != ?", phoneStr, id).First(&existingMember).Error; err == nil {
				return fmt.Errorf("phone %s is already in use", phoneStr)
			} else if err != gorm.ErrRecordNotFound {
				log.Printf("Failed to check for duplicate phone: %v", err)
				return fmt.Errorf("failed to check for duplicate phone: %w", err)
			}
			mappedFields["phone"] = phoneStr
		case "password":
			passwordStr, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid password type: must be a string")
			}
			// 驗證密碼（至少 8 個字元，包含字母和數字）
			if len(passwordStr) < 8 ||
				!regexp.MustCompile(`[a-zA-Z]`).MatchString(passwordStr) ||
				!regexp.MustCompile(`[0-9]`).MatchString(passwordStr) {
				return fmt.Errorf("password must be at least 8 characters and include both letters and numbers")
			}
			hashedPassword, err := utils.HashPassword(passwordStr)
			if err != nil {
				return fmt.Errorf("failed to hash password: %w", err)
			}
			mappedFields["password"] = hashedPassword
		case "role":
			roleStr, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid role type: must be a string")
			}
			if roleStr != "renter" && roleStr != "admin" {
				return fmt.Errorf("invalid role: must be 'renter' or 'admin'")
			}
			mappedFields["role"] = roleStr
		case "payment_info":
			paymentInfoStr, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid payment_info type: must be a string")
			}
			encryptedPaymentInfo, err := utils.EncryptPaymentInfo(paymentInfoStr)
			if err != nil {
				return fmt.Errorf("failed to encrypt payment_info: %w", err)
			}
			mappedFields["payment_info"] = encryptedPaymentInfo
		case "license_plate":
			mappedFields["license_plate"] = value
		case "email":
			emailStr, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid email type: must be a string")
			}
			// 檢查是否有重複的 email
			var existingMember models.Member
			if err := database.DB.Where("email = ? AND member_id != ?", emailStr, id).First(&existingMember).Error; err == nil {
				return fmt.Errorf("email %s is already in use", emailStr)
			} else if err != gorm.ErrRecordNotFound {
				log.Printf("Failed to check for duplicate email: %v", err)
				return fmt.Errorf("failed to check for duplicate email: %w", err)
			}
			mappedFields["email"] = emailStr
		default:
			return fmt.Errorf("invalid field: %s", key)
		}
	}

	// 使用 GORM 更新數據
	if err := database.DB.Model(&member).Updates(mappedFields).Error; err != nil {
		log.Printf("Failed to update member with fields %v: %v", mappedFields, err)
		return fmt.Errorf("failed to update member with ID %d: %w", id, err)
	}

	log.Printf("Successfully updated member with ID %d", id)
	return nil
}

// DeleteMember 刪除會員
func DeleteMember(id int) error {
	// Start a transaction
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred during member deletion: %v", r)
		}
	}()

	var member models.Member
	if err := tx.First(&member, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with ID %d not found", id)
			tx.Rollback()
			return fmt.Errorf("member with ID %d not found", id)
		}
		log.Printf("Failed to find member: %v", err)
		tx.Rollback()
		return fmt.Errorf("failed to find member with ID %d: %w", id, err)
	}

	// 刪除相關的 rents
	if err := tx.Where("member_id = ?", id).Delete(&models.Rent{}).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to delete rents for member %d: %v", id, err)
		return fmt.Errorf("failed to delete rents for member %d: %w", id, err)
	}

	// 提交事務
	if err := tx.Delete(&member).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to delete member: %v", err)
		return fmt.Errorf("failed to delete member with ID %d: %w", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction for member deletion: %v", err)
		return fmt.Errorf("failed to commit transaction for member deletion: %w", err)
	}

	log.Printf("Successfully deleted member with ID %d", id)
	return nil
}

// GetMemberRentHistory 查詢特定會員的租賃歷史記錄
func GetMemberRentHistory(memberID int) ([]models.Rent, error) {
	var rents []models.Rent
	if err := database.DB.
		Preload("ParkingSpot").
		Where("member_id = ?", memberID).
		Find(&rents).Error; err != nil {
		log.Printf("Failed to get rent history for member %d: %v", memberID, err)
		return nil, fmt.Errorf("failed to get rent history for member %d: %w", memberID, err)
	}

	log.Printf("Successfully retrieved %d rent records for member %d", len(rents), memberID)
	return rents, nil
}

// UpdateLicensePlate 更新會員的車牌號碼
func UpdateLicensePlate(memberID int, licensePlate string) error {
	// 驗證車牌格式（例如：X-XXXX 或 XX-XXXX）
	if match, _ := regexp.MatchString(`^[A-Z]{1,3}-[0-9]{4}$`, licensePlate); !match {
		return fmt.Errorf("invalid license plate format: must be X-XXXX or XX-XXXX")
	}

	// 查詢會員
	var member models.Member
	if err := database.DB.First(&member, memberID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("member with ID %d not found", memberID)
		}
		return fmt.Errorf("failed to query member with ID %d: %w", memberID, err)
	}

	// 更新車牌
	member.LicensePlate = licensePlate
	if err := database.DB.Save(&member).Error; err != nil {
		log.Printf("Failed to update license plate for member %d: %v", memberID, err)
		return fmt.Errorf("failed to update license plate for member %d: %w", memberID, err)
	}

	log.Printf("Successfully updated license plate for member %d to %s", memberID, licensePlate)
	return nil
}
