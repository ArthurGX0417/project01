package services

import (
	"encoding/base64"
	"fmt"
	"log"
	"project01/database"
	"project01/models"
	"project01/utils"

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

	// 驗證 payment_method 和 role
	if member.PaymentMethod != "credit_card" && member.PaymentMethod != "e_wallet" {
		return fmt.Errorf("invalid payment_method: must be 'credit_card' or 'e_wallet'")
	}
	if member.Role != "shared_owner" && member.Role != "renter" {
		return fmt.Errorf("invalid role: must be 'shared_owner' or 'renter'")
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
			return nil, fmt.Errorf("無效的電子郵件/電話或密碼")
		}
		log.Printf("Failed to login member: %v", err)
		return nil, fmt.Errorf("failed to login member: %w", err)
	}

	// 驗證密碼
	if !utils.CheckPasswordHash(password, member.Password) {
		log.Printf("Invalid password for email %s or phone %s", email, phone)
		return nil, fmt.Errorf("無效的電子郵件/電話或密碼")
	}

	log.Printf("Member with ID %d logged in successfully", member.MemberID)
	return &member, nil
}

// GetMemberByID 根據ID查詢會員
func GetMemberByID(id int) (*models.Member, error) {
	var member models.Member
	if err := database.DB.
		Preload("Spots").               // 修正為 Spots
		Preload("Spots.AvailableDays"). // 預載停車位的可用日期
		Preload("Rents").
		First(&member, id).Error; err != nil {
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

// GetAllMembers 查詢所有會員
func GetAllMembers() ([]models.Member, error) {
	var members []models.Member
	if err := database.DB.
		Preload("Spots").
		Preload("Spots.AvailableDays").
		Preload("Rents").
		Find(&members).Error; err != nil {
		log.Printf("Failed to query all members: %v", err)
		return nil, fmt.Errorf("failed to query all members: %w", err)
	}

	for i := range members {
		if members[i].PaymentInfo != "" {
			_, err := base64.StdEncoding.DecodeString(members[i].PaymentInfo)
			if err != nil {
				log.Printf("payment_info for member %d is not a valid Base64 string: %v", members[i].MemberID, err)
				continue
			}

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
		case "name":
			mappedFields["name"] = value
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
			if roleStr != "shared_owner" && roleStr != "renter" {
				return fmt.Errorf("invalid role: must be 'shared_owner' or 'renter'")
			}
			mappedFields["role"] = roleStr
		case "payment_method":
			paymentMethodStr, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid payment_method type: must be a string")
			}
			if paymentMethodStr != "credit_card" && paymentMethodStr != "e_wallet" {
				return fmt.Errorf("invalid payment_method: must be 'credit_card' or 'e_wallet'")
			}
			mappedFields["payment_method"] = paymentMethodStr
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
		case "auto_monthly_payment":
			autoMonthlyPayment, ok := value.(bool)
			if !ok {
				return fmt.Errorf("invalid auto_monthly_payment type: must be a boolean")
			}
			mappedFields["auto_monthly_payment"] = autoMonthlyPayment
		case "license_plate":
			mappedFields["license_plate"] = value
		case "car_model":
			mappedFields["car_model"] = value
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
		case "wifi_verified":
			wifiVerified, ok := value.(bool)
			if !ok {
				return fmt.Errorf("invalid wifi_verified type: must be a boolean")
			}
			mappedFields["wifi_verified"] = wifiVerified
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

	// 查找所有相關的 parking_spots
	var parkingSpots []models.ParkingSpot
	if err := tx.Where("member_id = ?", id).Find(&parkingSpots).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to find parking spots for member %d: %v", id, err)
		return fmt.Errorf("failed to find parking spots for member %d: %w", id, err)
	}

	// 收集所有 parking_spot 的 ID
	spotIDs := make([]int, len(parkingSpots))
	for i, spot := range parkingSpots {
		spotIDs[i] = spot.SpotID
	}

	// 刪除相關的 parking_spot_available_days
	if len(spotIDs) > 0 {
		if err := tx.Where("parking_spot_id IN ?", spotIDs).Delete(&models.ParkingSpotAvailableDay{}).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to delete parking spot available days for member %d: %v", id, err)
			return fmt.Errorf("failed to delete parking spot available days for member %d: %w", id, err)
		}
	}

	// 刪除相關的 parking_spots
	if err := tx.Where("member_id = ?", id).Delete(&models.ParkingSpot{}).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to delete parking spots for member %d: %v", id, err)
		return fmt.Errorf("failed to delete parking spots for member %d: %w", id, err)
	}

	// 刪除會員
	if err := tx.Delete(&member).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to delete member: %v", err)
		return fmt.Errorf("failed to delete member with ID %d: %w", id, err)
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction for member deletion: %v", err)
		return fmt.Errorf("failed to commit transaction for member deletion: %w", err)
	}

	log.Printf("Successfully deleted member with ID %d", id)
	return nil
}

// VerifyWifi Wifi驗證
func VerifyWifi(memberID int) (bool, error) {
	var member models.Member
	// 使用 GORM 查詢 wifi_verified
	if err := database.DB.Select("wifi_verified").First(&member, memberID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with ID %d not found", memberID)
			return false, fmt.Errorf("member with ID %d not found", memberID)
		}
		log.Printf("Failed to verify WiFi for member %d: %v", memberID, err)
		return false, fmt.Errorf("failed to verify WiFi for member %d: %w", memberID, err)
	}

	log.Printf("Successfully verified WiFi for member %d: %v", memberID, member.WifiVerified)
	return member.WifiVerified, nil
}

// GetMemberRentHistory 查詢特定會員的租賃歷史記錄
func GetMemberRentHistory(memberID int) ([]models.Rent, error) {
	var rents []models.Rent
	if err := database.DB.
		Preload("ParkingSpot").
		Preload("ParkingSpot.Member").
		Preload("Member").
		Where("member_id = ?", memberID).
		Find(&rents).Error; err != nil {
		log.Printf("Failed to get rent history for member %d: %v", memberID, err)
		return nil, fmt.Errorf("failed to get rent history for member %d: %w", memberID, err)
	}

	log.Printf("Successfully retrieved %d rent records for member %d", len(rents), memberID)
	return rents, nil
}
