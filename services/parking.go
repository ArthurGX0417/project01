// services/parking.go
package services

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"project01/database"
	"project01/models"
	"time"

	"gorm.io/gorm"
)

// ShareParkingSpot 共享停車位
func ShareParkingSpot(spot *models.ParkingSpot) error {
	// 驗證 ENUM 值
	if spot.ParkingType != "mechanical" && spot.ParkingType != "flat" {
		return fmt.Errorf("invalid parking_type: must be 'mechanical' or 'flat'")
	}
	if spot.PricingType != "monthly" && spot.PricingType != "hourly" {
		return fmt.Errorf("invalid pricing_type: must be 'monthly' or 'hourly'")
	}
	// 如果未設置 status，設置預設值為 "idle"
	if spot.Status == "" {
		spot.Status = "idle"
	}
	if spot.Status != "in_use" && spot.Status != "idle" {
		return fmt.Errorf("invalid status: must be 'in_use' or 'idle'")
	}

	// 驗證 member_id 是否存在
	var member models.Member
	if err := database.DB.Where("member_id = ?", spot.MemberID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("member with ID %d not found", spot.MemberID)
		}
		log.Printf("Failed to verify member: %v", err)
		return fmt.Errorf("failed to verify member: %w", err)
	}

	// 驗證 member 是否為 shared_owner
	if member.Role != "shared_owner" {
		return fmt.Errorf("only shared_owner can share parking spots")
	}

	// 設置費率預設值
	if spot.PricePerHalfHour == 0 {
		spot.PricePerHalfHour = 20
	}
	if spot.DailyMaxPrice == 0 {
		spot.DailyMaxPrice = 300
	}

	// 將 available_days 序列化為 JSON 字符串
	var availableDaysJSON string
	if spot.AvailableDays != "" {
		var days []string
		if err := json.Unmarshal([]byte(spot.AvailableDays), &days); err != nil {
			return fmt.Errorf("invalid available_days format: %v", err)
		}
		validDays := map[string]bool{
			"Monday": true, "Tuesday": true, "Wednesday": true, "Thursday": true,
			"Friday": true, "Saturday": true, "Sunday": true,
		}
		for _, day := range days {
			if !validDays[day] {
				return fmt.Errorf("invalid day in available_days: %s", day)
			}
		}
		availableDaysJSON = spot.AvailableDays
	} else {
		// 如果未設置，預設為全週可用
		defaultDays := []string{"Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday", "Sunday"}
		daysJSON, err := json.Marshal(defaultDays)
		if err != nil {
			return fmt.Errorf("failed to marshal default available_days: %v", err)
		}
		availableDaysJSON = string(daysJSON)
	}
	// 將 JSON 字符串存儲到 AvailableDays 字段
	spot.AvailableDays = availableDaysJSON

	// 使用 GORM 插入車位
	if err := database.DB.Create(spot).Error; err != nil {
		log.Printf("Failed to share parking spot: %v", err)
		return fmt.Errorf("failed to create parking spot: %w", err)
	}

	log.Printf("Successfully shared parking spot with ID %d", spot.SpotID)
	return nil
}

// GetAvailableParkingSpots 查詢可用停車位
func GetAvailableParkingSpots() ([]models.ParkingSpot, error) {
	var spots []models.ParkingSpot
	currentDay := time.Now().Weekday().String()

	// 查詢 status 為 idle 且當前星期在 available_days 中的車位
	if err := database.DB.
		Preload("Member").
		Preload("Rents", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Member").Preload("ParkingSpot", func(db *gorm.DB) *gorm.DB {
				return db.Preload("Member")
			})
		}).
		Where("status = ? AND NOT EXISTS (SELECT 1 FROM rents WHERE rents.spot_id = parking_spots.spot_id AND rents.actual_end_time IS NULL)", "idle").
		Find(&spots).Error; err != nil {
		log.Printf("Failed to query available parking spots: %v", err)
		return nil, err
	}

	// 過濾掉當前時間不在 available_days 中的車位
	var availableSpots []models.ParkingSpot
	for _, spot := range spots {
		var days []string
		if spot.AvailableDays != "" {
			if err := json.Unmarshal([]byte(spot.AvailableDays), &days); err != nil {
				log.Printf("Failed to parse available_days for spot %d: %v", spot.SpotID, err)
				continue
			}
		}

		// 檢查當前星期是否在可用日期中
		isAvailable := false
		for _, day := range days {
			if day == currentDay {
				isAvailable = true
				break
			}
		}

		if isAvailable {
			availableSpots = append(availableSpots, spot)
		}
	}

	return availableSpots, nil
}

// 查詢特定車位
func GetParkingSpotByID(id int) (*models.ParkingSpot, error) {
	var spot models.ParkingSpot
	// 添加嵌套 Preload
	if err := database.DB.
		Preload("Member").
		Preload("Rents").
		Preload("Rents.Member").
		Preload("Rents.ParkingSpot").
		Preload("Rents.ParkingSpot.Member").
		First(&spot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot with ID %d not found", id)
			return nil, nil
		}
		log.Printf("Failed to get parking spot by ID %d: %v", id, err)
		return nil, fmt.Errorf("database error: %w", err)
	}
	return &spot, nil
}

// UpdateParkingSpot 更新車位信息
func UpdateParkingSpot(id int, updatedFields map[string]interface{}) error {
	var spot models.ParkingSpot
	if err := database.DB.First(&spot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("parking spot with ID %d not found", id)
		}
		log.Printf("Failed to find parking spot: %v", err)
		return err
	}

	// 驗證更新權限（僅限 shared_owner）
	var member models.Member
	if err := database.DB.Where("member_id = ?", spot.MemberID).First(&member).Error; err != nil {
		log.Printf("Failed to verify member: %v", err)
		return fmt.Errorf("failed to verify member: %w", err)
	}
	if member.Role != "shared_owner" {
		return fmt.Errorf("only shared_owner can update parking spot")
	}

	// 映射並驗證字段
	mappedFields := make(map[string]interface{})
	for key, value := range updatedFields {
		switch key {
		case "price_per_half_hour":
			price, ok := value.(float64)
			if !ok {
				return fmt.Errorf("invalid price_per_half_hour type: must be a number")
			}
			if price <= 0 {
				return fmt.Errorf("price_per_half_hour must be positive")
			}
			mappedFields["price_per_half_hour"] = price
		case "daily_max_price":
			maxPrice, ok := value.(float64)
			if !ok {
				return fmt.Errorf("invalid daily_max_price type: must be a number")
			}
			if maxPrice <= 0 {
				return fmt.Errorf("daily_max_price must be positive")
			}
			mappedFields["daily_max_price"] = maxPrice
		case "location":
			location, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid location type: must be a string")
			}
			mappedFields["location"] = location
		case "longitude":
			long, ok := value.(float64)
			if !ok {
				return fmt.Errorf("invalid longitude type: must be a number")
			}
			mappedFields["longitude"] = long
		case "latitude":
			lat, ok := value.(float64)
			if !ok {
				return fmt.Errorf("invalid latitude type: must be a number")
			}
			mappedFields["latitude"] = lat
		case "available_days":
			days, ok := value.([]interface{})
			if !ok {
				return fmt.Errorf("invalid available_days type: must be an array")
			}
			var dayStrings []string
			for _, day := range days {
				dayStr, ok := day.(string)
				if !ok {
					return fmt.Errorf("invalid day in available_days: must be a string")
				}
				validDays := map[string]bool{
					"Monday":    true,
					"Tuesday":   true,
					"Wednesday": true,
					"Thursday":  true,
					"Friday":    true,
					"Saturday":  true,
					"Sunday":    true,
				}
				if !validDays[dayStr] {
					return fmt.Errorf("invalid day in available_days: %s", dayStr)
				}
				dayStrings = append(dayStrings, dayStr)
			}
			daysJSON, err := json.Marshal(dayStrings)
			if err != nil {
				return fmt.Errorf("failed to marshal available_days: %v", err)
			}
			mappedFields["available_days"] = string(daysJSON)
		default:
			return fmt.Errorf("invalid field: %s", key)
		}
	}

	if len(mappedFields) == 0 {
		return fmt.Errorf("no valid fields to update")
	}

	if err := database.DB.Model(&spot).Updates(mappedFields).Error; err != nil {
		log.Printf("Failed to update parking spot with ID %d: %v", id, err)
		return err
	}
	return nil
}
