package services

import (
	"errors"
	"fmt"
	"log"
	"project01/database"
	"project01/models"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

// FetchAvailableDays 取得停車位的可用天數
func FetchAvailableDays(spotID int) ([]models.ParkingSpotAvailableDay, error) {
	var availableDays []models.ParkingSpotAvailableDay
	if err := database.DB.Where("parking_spot_id = ?", spotID).Find(&availableDays).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch available days for spot %d: %w", spotID, err)
	}
	return availableDays, nil
}

// ShareParkingSpot 共享停車位
func ShareParkingSpot(spot *models.ParkingSpot, availableDays []models.ParkingSpotAvailableDay) error {
	if spot.ParkingType != "mechanical" && spot.ParkingType != "flat" {
		return fmt.Errorf("invalid parking_type: must be 'mechanical' or 'flat'")
	}
	if spot.PricingType != "monthly" && spot.PricingType != "hourly" {
		return fmt.Errorf("invalid pricing_type: must be 'monthly' or 'hourly'")
	}
	if spot.Status != "available" && spot.Status != "occupied" && spot.Status != "reserved" {
		return fmt.Errorf("invalid status: must be 'available', 'occupied', or 'reserved'")
	}

	// 檢查日期重複性（AvailableDate 現在是 time.Time）
	seenDates := make(map[string]bool)
	for _, day := range availableDays {
		// 將 time.Time 格式化為 YYYY-MM-DD 進行重複檢查
		dateStr := day.AvailableDate.Format("2006-01-02")
		if seenDates[dateStr] {
			return fmt.Errorf("duplicate date in available_days: %s", dateStr)
		}
		seenDates[dateStr] = true
	}

	var member models.Member
	if err := database.DB.Where("member_id = ?", spot.MemberID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("member with ID %d not found", spot.MemberID)
		}
		log.Printf("Failed to verify member: %v", err)
		return fmt.Errorf("failed to verify member: %w", err)
	}

	if member.Role != "shared_owner" {
		return fmt.Errorf("only shared_owner can share parking spots")
	}

	tx := database.DB.Begin()

	if err := tx.Create(spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to share parking spot: %v", err)
		return fmt.Errorf("failed to create parking spot: %w", err)
	}

	// 如果提供了 availableDays，則創建可用日期記錄
	for _, day := range availableDays {
		day.SpotID = spot.SpotID
		if err := tx.Create(&day).Error; err != nil {
			tx.Rollback()
			if gormErr, ok := err.(*mysql.MySQLError); ok && gormErr.Number == 1062 {
				return fmt.Errorf("duplicate entry for spot_id %d and date %s", spot.SpotID, day.AvailableDate.Format("2006-01-02"))
			}
			return fmt.Errorf("failed to insert available date %s: %w", day.AvailableDate.Format("2006-01-02"), err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully shared parking spot with ID %d", spot.SpotID)
	return nil
}

// GetAvailableParkingSpots 查詢可用停車位
func GetAvailableParkingSpots(location, date string) ([]models.ParkingSpot, [][]models.ParkingSpotAvailableDay, error) {
	var spots []models.ParkingSpot

	// 構建查詢
	query := database.DB.
		Joins("INNER JOIN parking_spot_available_day pad ON parking_spot.spot_id = pad.parking_spot_id").
		Where("pad.is_available = ?", true).
		Where("NOT EXISTS (SELECT 1 FROM rent WHERE rent.spot_id = parking_spot.spot_id AND rent.actual_end_time IS NULL)")

	// 如果提供了 location，添加過濾條件
	if location != "" {
		query = query.Where("parking_spot.location LIKE ?", "%"+location+"%")
	}

	// 如果提供了 date，添加過濾條件；否則使用當前日期
	if date != "" {
		query = query.Where("pad.available_date = ?", date)
	} else {
		query = query.Where("pad.available_date = ?", time.Now().Format("2006-01-02"))
	}

	// 確保停車位狀態為 available
	query = query.Where("parking_spot.status = ?", "available")

	// 執行查詢
	err := query.Find(&spots).Error
	if err != nil {
		log.Printf("Failed to query available parking spots: %v", err)
		return nil, nil, fmt.Errorf("failed to query available parking spots: %w", err)
	}

	// 如果沒有找到任何停車位，返回空陣列
	if len(spots) == 0 {
		return spots, nil, nil
	}

	// 提取 spotIDs
	spotIDs := make([]int, len(spots))
	for i, spot := range spots {
		spotIDs[i] = spot.SpotID
	}

	// 查詢可用日期
	var availableDaysRecords []models.ParkingSpotAvailableDay
	if err := database.DB.Where("parking_spot_id IN ?", spotIDs).Find(&availableDaysRecords).Error; err != nil {
		log.Printf("Failed to fetch available days for spots: %v", err)
		availableDaysRecords = []models.ParkingSpotAvailableDay{}
	}

	// 將可用日期按 spotID 分組
	availableDaysMap := make(map[int][]models.ParkingSpotAvailableDay)
	for _, record := range availableDaysRecords {
		availableDaysMap[record.SpotID] = append(availableDaysMap[record.SpotID], record)
	}

	// 為每個停車位分配可用日期
	availableDaysList := make([][]models.ParkingSpotAvailableDay, len(spots))
	for i, spot := range spots {
		availableDaysList[i] = availableDaysMap[spot.SpotID]
		if availableDaysList[i] == nil {
			availableDaysList[i] = []models.ParkingSpotAvailableDay{}
		}
	}

	log.Printf("Successfully retrieved %d available parking spots", len(spots))
	return spots, availableDaysList, nil
}

// GetParkingSpotByID 查詢特定車位
func GetParkingSpotByID(id int) (*models.ParkingSpot, []models.ParkingSpotAvailableDay, error) {
	var spot models.ParkingSpot
	if err := database.DB.
		Preload("Member").
		Preload("Rents"). // 應載入所有 Rents
		First(&spot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot with ID %d not found", id)
			return nil, nil, nil
		}
		log.Printf("Failed to get parking spot by ID %d: %v", id, err)
		return nil, nil, fmt.Errorf("failed to get parking spot by ID %d: %w", id, err)
	}

	days, err := FetchAvailableDays(spot.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", spot.SpotID, err)
		days = []models.ParkingSpotAvailableDay{}
	}

	log.Printf("Successfully retrieved parking spot with ID %d", id)
	return &spot, days, nil
}

// UpdateParkingSpot 更新車位信息
func UpdateParkingSpot(id int, updatedFields map[string]interface{}) error {
	var spot models.ParkingSpot
	if err := database.DB.First(&spot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot with ID %d not found", id)
			return fmt.Errorf("parking spot with ID %d not found", id)
		}
		log.Printf("Failed to find parking spot: %v", err)
		return fmt.Errorf("failed to find parking spot with ID %d: %w", id, err)
	}

	var member models.Member
	if err := database.DB.Where("member_id = ?", spot.MemberID).First(&member).Error; err != nil {
		log.Printf("Failed to verify member: %v", err)
		return fmt.Errorf("failed to verify member: %w", err)
	}
	if member.Role != "shared_owner" {
		return fmt.Errorf("only shared_owner can update parking spot")
	}

	mappedFields := make(map[string]interface{})
	for key, value := range updatedFields {
		switch key {
		case "parking_type":
			parkingType, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid parking_type type: must be a string")
			}
			if parkingType != "mechanical" && parkingType != "flat" {
				return fmt.Errorf("invalid parking_type: must be 'mechanical' or 'flat'")
			}
			mappedFields["parking_type"] = parkingType
		case "pricing_type":
			pricingType, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid pricing_type type: must be a string")
			}
			if pricingType != "monthly" && pricingType != "hourly" {
				return fmt.Errorf("invalid pricing_type: must be 'monthly' or 'hourly'")
			}
			mappedFields["pricing_type"] = pricingType
		case "status":
			status, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid status type: must be a string")
			}
			if status != "in_use" && status != "idle" {
				return fmt.Errorf("invalid status: must be 'in_use' or 'idle'")
			}
			mappedFields["status"] = status
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
			var dayInputs []AvailableDayInput
			for _, day := range days {
				dayMap, ok := day.(map[string]interface{})
				if !ok {
					return fmt.Errorf("invalid day in available_days: must be an object")
				}
				date, ok := dayMap["date"].(string)
				if !ok {
					return fmt.Errorf("invalid date in available_days: must be a string")
				}
				isAvailable, ok := dayMap["is_available"].(bool)
				if !ok {
					return fmt.Errorf("invalid is_available in available_days: must be a boolean")
				}
				if _, err := time.Parse("2006-01-02", date); err != nil {
					return fmt.Errorf("invalid date format in available_days: %s", date)
				}
				dayInputs = append(dayInputs, AvailableDayInput{Date: date, IsAvailable: isAvailable})
			}

			tx := database.DB.Begin()
			if err := tx.Where("parking_spot_id = ?", id).Delete(&models.ParkingSpotAvailableDay{}).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to delete existing available days: %w", err)
			}
			for _, day := range dayInputs {
				// 將 string 類型的日期解析為 time.Time
				parsedDate, err := time.Parse("2006-01-02", day.Date)
				if err != nil {
					tx.Rollback()
					return fmt.Errorf("failed to parse date %s: %w", day.Date, err)
				}
				if err := tx.Create(&models.ParkingSpotAvailableDay{
					SpotID:        id,
					AvailableDate: parsedDate, // 使用解析後的 time.Time
					IsAvailable:   day.IsAvailable,
				}).Error; err != nil {
					tx.Rollback()
					if gormErr, ok := err.(*mysql.MySQLError); ok && gormErr.Number == 1062 {
						return fmt.Errorf("duplicate entry for spot_id %d and date %s", id, day.Date)
					}
					return fmt.Errorf("failed to insert available date %s: %w", day.Date, err)
				}
			}
			if err := tx.Commit().Error; err != nil {
				return fmt.Errorf("failed to commit transaction for available days: %w", err)
			}
		default:
			return fmt.Errorf("invalid field: %s", key)
		}
	}

	if len(mappedFields) == 0 {
		return fmt.Errorf("no valid fields to update")
	}

	if err := database.DB.Model(&spot).Updates(mappedFields).Error; err != nil {
		log.Printf("Failed to update parking spot with ID %d: %v", id, err)
		return fmt.Errorf("failed to update parking spot with ID %d: %w", id, err)
	}

	log.Printf("Successfully updated parking spot with ID %d", id)
	return nil
}

type AvailableDayInput struct {
	Date        string `json:"date"`
	IsAvailable bool   `json:"is_available"`
}
