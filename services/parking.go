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

// FetchAvailableDays fetches the available days for a parking spot
func FetchAvailableDays(spotID int) ([]string, error) {
	var availableDays []models.ParkingSpotAvailableDay
	if err := database.DB.Where("spot_id = ?", spotID).Find(&availableDays).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch available days for spot %d: %w", spotID, err)
	}
	days := make([]string, len(availableDays))
	for i, day := range availableDays {
		days[i] = day.Day
	}
	return days, nil
}

// ShareParkingSpot 共享停車位
func ShareParkingSpot(spot *models.ParkingSpot, availableDays []string) error {
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

	// 驗證 available_days
	if len(availableDays) == 0 {
		return fmt.Errorf("available_days cannot be empty")
	}
	seenDays := make(map[string]bool)
	for _, day := range availableDays {
		validDays := map[string]bool{
			"Monday":    true,
			"Tuesday":   true,
			"Wednesday": true,
			"Thursday":  true,
			"Friday":    true,
			"Saturday":  true,
			"Sunday":    true,
		}
		if !validDays[day] {
			return fmt.Errorf("invalid day in available_days: %s", day)
		}
		if seenDays[day] {
			return fmt.Errorf("duplicate day in available_days: %s", day)
		}
		seenDays[day] = true
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

	// Start a transaction
	tx := database.DB.Begin()

	// 使用 GORM 插入車位
	if err := tx.Create(spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to share parking spot: %v", err)
		return fmt.Errorf("failed to create parking spot: %w", err)
	}

	// Insert available days into parking_spot_available_days
	for _, day := range availableDays {
		if err := tx.Create(&models.ParkingSpotAvailableDay{SpotID: spot.SpotID, Day: day}).Error; err != nil {
			tx.Rollback()
			// Check if the error is due to a duplicate key (unique constraint violation)
			if gormErr, ok := err.(*mysql.MySQLError); ok && gormErr.Number == 1062 {
				return fmt.Errorf("duplicate entry for spot_id %d and day %s", spot.SpotID, day)
			}
			return fmt.Errorf("failed to insert available day %s: %w", day, err)
		}
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully shared parking spot with ID %d", spot.SpotID)
	return nil
}

// GetAvailableParkingSpots 查詢可用停車位
func GetAvailableParkingSpots() ([]models.ParkingSpot, [][]string, error) {
	var spots []models.ParkingSpot
	currentDay := time.Now().Weekday().String()

	// 查詢 status 為 idle 且當前星期在 parking_spot_available_days 中的車位
	err := database.DB.
		Joins("INNER JOIN parking_spot_available_days pad ON parking_spots.spot_id = pad.spot_id").
		Where("parking_spots.status = ? AND pad.day = ?", "idle", currentDay).
		Where("NOT EXISTS (SELECT 1 FROM rents WHERE rents.spot_id = parking_spots.spot_id AND rents.actual_end_time IS NULL)").
		Find(&spots).Error

	if err != nil {
		log.Printf("Failed to query available parking spots: %v", err)
		return nil, nil, fmt.Errorf("failed to query available parking spots: %w", err)
	}

	// Fetch all available days for all spots in a single query
	spotIDs := make([]int, len(spots))
	for i, spot := range spots {
		spotIDs[i] = spot.SpotID
	}

	var availableDaysRecords []models.ParkingSpotAvailableDay
	if err := database.DB.Where("spot_id IN ?", spotIDs).Find(&availableDaysRecords).Error; err != nil {
		log.Printf("Failed to fetch available days for spots: %v", err)
		// Continue with empty days to avoid failing the entire request
		availableDaysRecords = []models.ParkingSpotAvailableDay{}
	}

	// Group available days by spot_id
	availableDaysMap := make(map[int][]string)
	for _, record := range availableDaysRecords {
		availableDaysMap[record.SpotID] = append(availableDaysMap[record.SpotID], record.Day)
	}

	// Populate availableDaysList
	availableDaysList := make([][]string, len(spots))
	for i, spot := range spots {
		availableDaysList[i] = availableDaysMap[spot.SpotID]
		if availableDaysList[i] == nil {
			availableDaysList[i] = []string{} // Ensure empty slice if no days found
		}
	}

	log.Printf("Successfully retrieved %d available parking spots", len(spots))
	return spots, availableDaysList, nil
}

// GetParkingSpotByID 查詢特定車位
func GetParkingSpotByID(id int) (*models.ParkingSpot, []string, error) {
	var spot models.ParkingSpot
	if err := database.DB.
		Preload("Member").
		Preload("Rents").
		First(&spot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot with ID %d not found", id)
			return nil, nil, nil
		}
		log.Printf("Failed to get parking spot by ID %d: %v", id, err)
		return nil, nil, fmt.Errorf("failed to get parking spot by ID %d: %w", id, err)
	}

	// Fetch available days
	days, err := FetchAvailableDays(spot.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", spot.SpotID, err)
		// Continue with empty days to avoid failing the entire request
		days = []string{}
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
			// Handle available_days separately by updating the parking_spot_available_days table
			days, ok := value.([]interface{})
			if !ok {
				return fmt.Errorf("invalid available_days type: must be an array")
			}
			var dayStrings []string
			seenDays := make(map[string]bool)
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
				// Check for duplicates in the input
				if seenDays[dayStr] {
					return fmt.Errorf("duplicate day in available_days: %s", dayStr)
				}
				seenDays[dayStr] = true
				dayStrings = append(dayStrings, dayStr)
			}

			// Start a transaction to update available days
			tx := database.DB.Begin()
			// Delete existing available days
			if err := tx.Exec("DELETE FROM parking_spot_available_days WHERE spot_id = ?", id).Error; err != nil {
				tx.Rollback()
				return fmt.Errorf("failed to delete existing available days: %w", err)
			}
			// Insert new available days
			for _, day := range dayStrings {
				if err := tx.Create(&models.ParkingSpotAvailableDay{SpotID: id, Day: day}).Error; err != nil {
					tx.Rollback()
					// Check if the error is due to a duplicate key (unique constraint violation)
					if gormErr, ok := err.(*mysql.MySQLError); ok && gormErr.Number == 1062 {
						return fmt.Errorf("duplicate entry for spot_id %d and day %s", id, day)
					}
					return fmt.Errorf("failed to insert available day %s: %w", day, err)
				}
			}
			// Commit the transaction
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
