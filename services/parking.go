package services

import (
	"errors"
	"fmt"
	"log"
	"project01/database"
	"project01/models"
	"regexp"
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
	// 驗證 parking_type
	if spot.ParkingType != "mechanical" && spot.ParkingType != "flat" {
		return fmt.Errorf("invalid parking_type: must be 'mechanical' or 'flat'")
	}

	// 驗證 pricing_type
	if spot.PricingType != "monthly" && spot.PricingType != "hourly" {
		return fmt.Errorf("invalid pricing_type: must be 'monthly' or 'hourly'")
	}

	// 驗證 status
	if spot.Status != "available" && spot.Status != "occupied" && spot.Status != "reserved" {
		return fmt.Errorf("invalid status: must be 'available', 'occupied', or 'reserved'")
	}

	// 驗證經緯度
	if spot.Latitude == 0 && spot.Longitude == 0 {
		return fmt.Errorf("invalid latitude and longitude: both cannot be 0")
	}
	if spot.Latitude < -90 || spot.Latitude > 90 {
		return fmt.Errorf("invalid latitude: must be between -90 and 90")
	}
	if spot.Longitude < -180 || spot.Longitude > 180 {
		return fmt.Errorf("invalid longitude: must be between -180 and 180")
	}

	// 設置價格預設值
	if spot.PricePerHalfHour == 0 && spot.PricingType == "hourly" {
		spot.PricePerHalfHour = 20.00
	}
	if spot.DailyMaxPrice == 0 && spot.PricingType == "hourly" {
		spot.DailyMaxPrice = 300.00
	}

	// 檢查日期重複性和有效性
	now := time.Now().UTC()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	seenDates := make(map[string]bool)
	for _, day := range availableDays {
		// 將 time.Time 格式化為 YYYY-MM-DD 進行重複檢查
		dateStr := day.AvailableDate.Format("2006-01-02")
		if seenDates[dateStr] {
			return fmt.Errorf("duplicate date in available_days: %s", dateStr)
		}
		seenDates[dateStr] = true

		// 確保日期是今天或未來的日期
		if day.AvailableDate.Before(today) {
			return fmt.Errorf("available date must be today or in the future: %s", dateStr)
		}
	}

	// 驗證會員
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

	// 開始事務
	tx := database.DB.Begin()

	// 創建停車位
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

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully shared parking spot with ID %d", spot.SpotID)
	return nil
}

// GetAvailableParkingSpots 查詢可用車位，基於日期和經緯度
func GetAvailableParkingSpots(date string, latitude, longitude, radius float64) ([]models.ParkingSpot, [][]models.ParkingSpotAvailableDay, error) {
	var spots []models.ParkingSpot

	// 驗證 radius 參數
	if radius <= 0 {
		radius = 3.0 // 預設值為 3 公里
	}
	if radius > 50 {
		radius = 50.0 // 最大值為 50 公里
	}

	// Haversine 公式計算距離（單位：公里）
	distanceSQL := `
        6371 * acos(
            cos(radians(?)) * cos(radians(parking_spot.latitude)) * 
            cos(radians(parking_spot.longitude) - radians(?)) + 
            sin(radians(?)) * sin(radians(parking_spot.latitude))
        )
    `

	// 計算日期範圍
	now := time.Now().UTC()
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid date format: %w", err)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if parsedDate.Before(today) {
		return nil, nil, fmt.Errorf("date must be today or in the future: %s", date)
	}

	startOfDay := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Nanosecond)

	// 查詢當前正在被租用的車位
	var rentedSpotIDs []int
	if err := database.DB.Model(&models.Rent{}).
		Select("spot_id").
		Where("(actual_end_time IS NULL AND end_time >= ?) OR (end_time > ? AND start_time < ?)", now, startOfDay, endOfDay).
		Where("status NOT IN (?)", []string{"canceled"}). // 排除已取消的租約
		Distinct().
		Scan(&rentedSpotIDs).Error; err != nil {
		log.Printf("Failed to query rented spot IDs: %v", err)
		return nil, nil, fmt.Errorf("failed to query rented spot IDs: %w", err)
	}

	// 構建查詢
	query := database.DB.
		Preload("Member").
		Preload("Rents", "end_time >= ? OR actual_end_time IS NULL", now).
		Joins("LEFT JOIN parking_spot_available_day pad ON parking_spot.spot_id = pad.parking_spot_id AND pad.available_date = ? AND pad.is_available = ?", date, true).
		Where("parking_spot.status = ?", "available").
		Where("parking_spot.latitude != 0 AND parking_spot.longitude != 0").
		Where(distanceSQL+" <= ?", latitude, longitude, latitude, radius)

	// 添加租賃過濾條件
	if len(rentedSpotIDs) > 0 {
		query = query.Where("parking_spot.spot_id NOT IN (?)", rentedSpotIDs)
	}

	// 執行查詢
	err = query.Find(&spots).Error
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
	if err := database.DB.Where("parking_spot_id IN ? AND available_date = ? AND is_available = ?", spotIDs, date, true).Find(&availableDaysRecords).Error; err != nil {
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

	log.Printf("Successfully retrieved %d available parking spots within %f km", len(spots), radius)
	return spots, availableDaysList, nil
}

// GetParkingSpotByID 查詢特定車位
func GetParkingSpotByID(id int) (*models.ParkingSpot, []models.ParkingSpotAvailableDay, error) {
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

	days, err := FetchAvailableDays(spot.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", spot.SpotID, err)
		days = []models.ParkingSpotAvailableDay{}
	}

	log.Printf("Successfully retrieved parking spot with ID %d", id)
	return &spot, days, nil
}

// 預編譯 floor_level 的正則表達式
var floorLevelRegex = regexp.MustCompile(`^(ground|([1-9][0-9]*[F])|(B[1-9][0-9]*))$`)

// UpdateParkingSpot 更新車位信息
func UpdateParkingSpot(id int, updatedFields map[string]interface{}) error {
	var spot models.ParkingSpot
	if err := database.DB.First(&spot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot with ID %d not found", id)
			return fmt.Errorf("parking spot with ID %d not found", id)
		}
		log.Printf("Failed to find parking spot with ID %d: %v", id, err)
		return fmt.Errorf("failed to find parking spot with ID %d: %w", id, err)
	}

	// 權限檢查移至處理器層，服務層僅處理更新邏輯

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
			if pricingType != "hourly" {
				return fmt.Errorf("invalid pricing_type: must be 'hourly'")
			}
			mappedFields["pricing_type"] = pricingType
		case "status":
			status, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid status type: must be a string")
			}
			if status != "available" && status != "occupied" && status != "reserved" {
				return fmt.Errorf("invalid status: must be 'available', 'occupied', or 'reserved'")
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
		case "floor_level":
			floorLevel, ok := value.(string)
			if !ok {
				return fmt.Errorf("invalid floor_level type: must be a string")
			}
			if len(floorLevel) > 20 {
				return fmt.Errorf("floor_level must not exceed 20 characters")
			}
			// 使用預編譯的正則表達式驗證 floor_level
			if !floorLevelRegex.MatchString(floorLevel) {
				return fmt.Errorf("invalid floor_level: must be 'ground', a number followed by 'F' (e.g., '1F'), or 'B' followed by a number (e.g., 'B1')")
			}
			mappedFields["floor_level"] = floorLevel
		case "available_days":
			days, ok := value.([]interface{})
			if !ok {
				return fmt.Errorf("invalid available_days type: must be an array")
			}
			var dayInputs []AvailableDayInput
			for i, day := range days {
				dayMap, ok := day.(map[string]interface{})
				if !ok {
					return fmt.Errorf("invalid day at index %d in available_days: must be an object", i)
				}
				date, ok := dayMap["date"].(string)
				if !ok {
					return fmt.Errorf("invalid date at index %d in available_days: must be a string", i)
				}
				isAvailable, ok := dayMap["is_available"].(bool)
				if !ok {
					return fmt.Errorf("invalid is_available at index %d in available_days: must be a boolean", i)
				}
				if _, err := time.Parse("2006-01-02", date); err != nil {
					return fmt.Errorf("invalid date format at index %d in available_days: %s", i, date)
				}
				dayInputs = append(dayInputs, AvailableDayInput{Date: date, IsAvailable: isAvailable})
			}

			tx := database.DB.Begin()
			if err := tx.Where("parking_spot_id = ?", id).Delete(&models.ParkingSpotAvailableDay{}).Error; err != nil {
				tx.Rollback()
				log.Printf("Failed to delete existing available days for spot %d: %v", id, err)
				return fmt.Errorf("failed to delete existing available days: %w", err)
			}
			now := time.Now().UTC()
			today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
			for i, day := range dayInputs {
				parsedDate, err := time.Parse("2006-01-02", day.Date)
				if err != nil {
					tx.Rollback()
					log.Printf("Failed to parse date at index %d: %v", i, err)
					return fmt.Errorf("failed to parse date %s: %w", day.Date, err)
				}
				if parsedDate.Before(today) {
					tx.Rollback()
					return fmt.Errorf("available date at index %d must be today or in the future: %s", i, day.Date)
				}
				if err := tx.Create(&models.ParkingSpotAvailableDay{
					SpotID:        id,
					AvailableDate: parsedDate,
					IsAvailable:   day.IsAvailable,
				}).Error; err != nil {
					tx.Rollback()
					if gormErr, ok := err.(*mysql.MySQLError); ok && gormErr.Number == 1062 {
						return fmt.Errorf("duplicate entry for spot_id %d and date %s", id, day.Date)
					}
					log.Printf("Failed to insert available date %s for spot %d: %v", day.Date, id, err)
					return fmt.Errorf("failed to insert available date %s: %w", day.Date, err)
				}
			}
			if err := tx.Commit().Error; err != nil {
				return fmt.Errorf("failed to commit transaction for available days: %w", err)
			}
		default:
			log.Printf("Ignoring invalid field: %s", key)
			continue
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

// GetMemberIncome 計算指定會員在指定時間範圍內的所有車位收入，並返回相關總計和記錄
func GetMemberIncome(memberID int, startDate, endDate time.Time, currentMemberID int, role string) (float64, []models.ParkingSpot, []models.Rent, error) {
	// 驗證角色
	validRoles := map[string]bool{"admin": true, "shared_owner": true}
	if !validRoles[role] {
		log.Printf("Invalid role: %s", role)
		return 0, nil, nil, fmt.Errorf("invalid role: %s", role)
	}

	// 檢查權限：admin 可以查看任何會員的收入，shared_owner 只能查看自己的收入
	if role != "admin" {
		if memberID != currentMemberID {
			log.Printf("Permission denied: member %d (role: %s) is not authorized to view income for member %d", currentMemberID, role, memberID)
			return 0, nil, nil, fmt.Errorf("permission denied: you can only view income of your own account")
		}
	}
	log.Printf("Permission granted: member %d (role: %s) can view income for member %d", currentMemberID, role, memberID)

	// 查詢該會員擁有的所有車位
	var spots []models.ParkingSpot
	if err := database.DB.Where("member_id = ?", memberID).Find(&spots).Error; err != nil {
		log.Printf("Failed to find parking spots for member %d: %v", memberID, err)
		return 0, nil, nil, fmt.Errorf("no parking spots found for member: %w", err)
	}
	if len(spots) == 0 {
		log.Printf("No parking spots found for member %d", memberID)
		return 0, spots, nil, nil // 返回空車位列表，收入為 0
	}

	// 查詢所有車位的租賃記錄
	var rents []models.Rent
	spotIDs := make([]int, len(spots))
	for i, spot := range spots {
		spotIDs[i] = spot.SpotID
	}
	if err := database.DB.Where("spot_id IN ? AND start_time >= ? AND start_time <= ? AND total_cost > 0", spotIDs, startDate, endDate).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for member %d's spots: %v", memberID, err)
		return 0, nil, nil, fmt.Errorf("failed to fetch rents: %w", err)
	}

	// 計算總收入
	var totalIncome float64
	if err := database.DB.Model(&models.Rent{}).
		Where("spot_id IN ? AND start_time >= ? AND start_time <= ? AND total_cost > 0", spotIDs, startDate, endDate).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&totalIncome).Error; err != nil {
		log.Printf("Failed to calculate income for member %d's spots: %v", memberID, err)
		return 0, nil, nil, fmt.Errorf("failed to calculate income: %w", err)
	}

	log.Printf("Calculated total income for member %d: %.2f, found %d rents across %d spots", memberID, totalIncome, len(rents), len(spots))
	return totalIncome, spots, rents, nil
}

// GetParkingSpotIncome 計算指定車位在指定時間範圍內的收入，並返回相關租賃記錄（保留用於單一車位查詢）
func GetParkingSpotIncome(spotID int, startDate, endDate time.Time, currentMemberID int, role string) (float64, *models.ParkingSpot, []models.Rent, error) {
	// 驗證角色
	validRoles := map[string]bool{"admin": true, "shared_owner": true}
	if !validRoles[role] {
		log.Printf("Invalid role: %s", role)
		return 0, nil, nil, fmt.Errorf("invalid role: %s", role)
	}

	// 查詢車位
	var spot models.ParkingSpot
	if err := database.DB.First(&spot, spotID).Error; err != nil {
		log.Printf("Failed to find parking spot %d: %v", spotID, err)
		return 0, nil, nil, fmt.Errorf("parking spot not found: %w", err)
	}

	// 檢查權限：admin 可以查看任何車位，shared_owner 只能查看自己的車位
	if role != "admin" {
		if spot.MemberID != currentMemberID {
			log.Printf("Permission denied: member %d (role: %s) is not the owner of spot %d (owned by member %d)", currentMemberID, role, spot.SpotID, spot.MemberID)
			return 0, nil, nil, fmt.Errorf("permission denied: you can only view income of your own parking spot")
		}
	}
	log.Printf("Permission granted: member %d (role: %s) can view income of spot %d", currentMemberID, role, spot.SpotID)

	// 查詢符合時間範圍的租賃記錄
	var rents []models.Rent
	if err := database.DB.Where("spot_id = ? AND start_time >= ? AND start_time <= ? AND total_cost > 0", spotID, startDate, endDate).Find(&rents).Error; err != nil {
		log.Printf("Failed to fetch rents for spot %d: %v", spotID, err)
		return 0, nil, nil, fmt.Errorf("failed to fetch rents: %w", err)
	}

	// 計算總收入
	var totalIncome float64
	if err := database.DB.Model(&models.Rent{}).
		Where("spot_id = ? AND start_time >= ? AND start_time <= ? AND total_cost > 0", spotID, startDate, endDate).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&totalIncome).Error; err != nil {
		log.Printf("Failed to calculate income for spot %d: %v", spotID, err)
		return 0, nil, nil, fmt.Errorf("failed to calculate income: %w", err)
	}

	log.Printf("Calculated total income for spot %d: %.2f, found %d rents", spot.SpotID, totalIncome, len(rents))
	return totalIncome, &spot, rents, nil
}

func DeleteParkingSpot(spotID int, currentMemberID int, role string) error {
	// 查詢車位
	var spot models.ParkingSpot
	if err := database.DB.First(&spot, spotID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot with ID %d not found", spotID)
			return fmt.Errorf("parking spot not found")
		}
		log.Printf("Failed to find parking spot %d: %v", spotID, err)
		return fmt.Errorf("failed to find parking spot: %w", err)
	}

	// 檢查權限：admin 可以刪除任何車位，shared_owner 只能刪除自己的車位
	if role != "admin" {
		if spot.MemberID != currentMemberID {
			log.Printf("Permission denied: member %d (role: %s) is not the owner of spot %d (owned by member %d)", currentMemberID, role, spot.SpotID, spot.MemberID)
			return fmt.Errorf("permission denied: you can only delete your own parking spot")
		}
	}

	// 開始事務
	tx := database.DB.Begin()

	// 刪除相關的可用日期記錄
	if err := tx.Where("parking_spot_id = ?", spotID).Delete(&models.ParkingSpotAvailableDay{}).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to delete available days for spot %d: %v", spotID, err)
		return fmt.Errorf("failed to delete available days: %w", err)
	}

	// 刪除車位
	if err := tx.Delete(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to delete parking spot %d: %v", spotID, err)
		return fmt.Errorf("failed to delete parking spot: %w", err)
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully deleted parking spot with ID %d", spotID)
	return nil
}

// GetMyParkingSpots 根據 member_id 查詢會員共享的所有車位
func GetMyParkingSpots(memberID int, role string) ([]models.ParkingSpotResponse, error) {
	var member models.Member
	if err := database.DB.Where("member_id = ?", memberID).First(&member).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, err
		}
		log.Printf("Database error while fetching member: %v", err)
		return nil, err
	}

	var parkingSpots []models.ParkingSpot
	query := database.DB.
		Preload("Member").
		Preload("AvailableDays").
		Preload("Rents", "actual_end_time IS NULL AND end_time > ?", time.Now().UTC())

	// 根據角色決定查詢條件
	if role != "admin" {
		query = query.Where("member_id = ?", memberID)
	}

	if err := query.Find(&parkingSpots).Error; err != nil {
		log.Printf("Database error while fetching parking spots: %v", err)
		return nil, err
	}

	// 轉換為回應格式
	var responses []models.ParkingSpotResponse
	for _, spot := range parkingSpots {
		response := spot.ToResponse(spot.AvailableDays, spot.Rents)
		responses = append(responses, response)
	}

	log.Printf("Found %d parking spots for member %d (role: %s)", len(parkingSpots), memberID, role)
	return responses, nil
}
