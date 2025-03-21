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

// 註冊會員
func RegisterMember(member *models.Member) error {
	// 驗證 payment_method 和 role
	if member.PaymentMethod != "credit_card" && member.PaymentMethod != "e_wallet" {
		return fmt.Errorf("invalid payment_method: must be 'credit_card' or 'e_wallet'")
	}
	if member.Role != "shared_owner" && member.Role != "renter" {
		return fmt.Errorf("invalid role: must be 'shared_owner' or 'renter'")
	}

	// 哈希密碼，雜湊
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
		return err
	}
	return nil
}

// 登入會員
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
		log.Printf("Failed to login member: %v", err)
		return nil, err
	}

	// 驗證密碼
	if !utils.CheckPasswordHash(password, member.Password) {
		log.Printf("Invalid password for email %s or phone %s", email, phone)
		return nil, fmt.Errorf("invalid password")
	}

	return &member, nil
}

// 根據ID查詢會員
func GetMemberByID(id int) (*models.Member, error) {
	var member models.Member
	if err := database.DB.
		Preload("ParkingSpots", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Member").Preload("Rents", func(db *gorm.DB) *gorm.DB {
				return db.Preload("Member").Preload("ParkingSpot", func(db *gorm.DB) *gorm.DB {
					return db.Preload("Member")
				})
			})
		}).
		Preload("Rents", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Member").Preload("ParkingSpot", func(db *gorm.DB) *gorm.DB {
				return db.Preload("Member")
			})
		}).
		First(&member, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with ID %d not found", id)
			return nil, nil
		}
		log.Printf("Failed to get member by ID: %v", err)
		return nil, err
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

	return &member, nil
}

// 查詢所有會員
func GetAllMembers() ([]models.Member, error) {
	var members []models.Member
	if err := database.DB.
		Preload("ParkingSpots", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Member").Preload("Rents", func(db *gorm.DB) *gorm.DB {
				return db.Preload("Member").Preload("ParkingSpot", func(db *gorm.DB) *gorm.DB {
					return db.Preload("Member")
				})
			})
		}).
		Preload("Rents", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Member").Preload("ParkingSpot", func(db *gorm.DB) *gorm.DB {
				return db.Preload("Member").Preload("Rents", func(db *gorm.DB) *gorm.DB {
					return db.Preload("Member").Preload("ParkingSpot", func(db *gorm.DB) *gorm.DB {
						return db.Preload("Member")
					})
				})
			})
		}).
		Find(&members).Error; err != nil {
		log.Printf("Failed to query all members: %v", err)
		return nil, err
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

	return members, nil
}

// 更新某ID會員資訊
func UpdateMember(id int, updatedFields map[string]interface{}) error {
	var member models.Member
	// 檢查會員是否存在
	if err := database.DB.First(&member, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return fmt.Errorf("member with ID %d not found", id)
		}
		log.Printf("Failed to find member: %v", err)
		return err
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
			mappedFields["phone"] = value
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
			mappedFields["email"] = value
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
		return err
	}
	return nil
}

// 刪除會員
func DeleteMember(id int) error {
	var member models.Member
	if err := database.DB.First(&member, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Member with ID %d not found", id)
			return fmt.Errorf("會員不存在")
		}
		log.Printf("Failed to find member: %v", err)
		return fmt.Errorf("查詢會員失敗: %v", err)
	}

	// 刪除相關的 rents
	if err := database.DB.Where("member_id = ?", id).Delete(&models.Rent{}).Error; err != nil {
		log.Printf("Failed to delete rents for member %d: %v", id, err)
		return fmt.Errorf("刪除租賃記錄失敗: %v", err)
	}

	// 刪除相關的 parking_spots
	if err := database.DB.Where("member_id = ?", id).Delete(&models.ParkingSpot{}).Error; err != nil {
		log.Printf("Failed to delete parking spots for member %d: %v", id, err)
		return fmt.Errorf("刪除停車位失敗: %v", err)
	}

	// 刪除會員
	if err := database.DB.Delete(&member).Error; err != nil {
		log.Printf("Failed to delete member: %v", err)
		return fmt.Errorf("刪除會員失敗: %v", err)
	}

	return nil
}

// Wifi驗證
func VerifyWifi(memberID int) (bool, error) {
	var member models.Member
	// 使用 GORM 查詢 wifi_verified
	if err := database.DB.Select("wifi_verified").First(&member, memberID).Error; err != nil {
		log.Printf("Failed to verify WiFi: %v", err)
		return false, err
	}
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
		return nil, err
	}
	return rents, nil
}
