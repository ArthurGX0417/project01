package services

import (
	"errors"
	"fmt"
	"log"
	"math"
	"project01/database"
	"project01/models"
	"time"

	"gorm.io/gorm"
)

// RentParkingSpot 租用車位
func RentParkingSpot(rent *models.Rent) error {
	var spot models.ParkingSpot
	// 驗證 member_id
	var member models.Member
	if err := database.DB.Where("member_id = ?", rent.MemberID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("member with ID %d not found", rent.MemberID)
		}
		log.Printf("Failed to verify member: %v", err)
		return fmt.Errorf("failed to verify member: %w", err)
	}

	// 檢查車位是否空閒
	if err := database.DB.Where("spot_id = ? AND status = ?", rent.SpotID, "idle").First(&spot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot %d is not idle", rent.SpotID)
			return fmt.Errorf("parking spot %d is not idle", rent.SpotID)
		}
		log.Printf("Failed to check parking spot status: %v", err)
		return fmt.Errorf("failed to check parking spot: %w", err)
	}

	// 使用 ParkingSpotAvailableDay 模型查詢可用日期
	var availableDays []models.ParkingSpotAvailableDay
	if err := database.DB.Where("spot_id = ?", spot.SpotID).Find(&availableDays).Error; err != nil {
		log.Printf("Failed to query available days for spot %d: %v", spot.SpotID, err)
		return fmt.Errorf("failed to query available days: %w", err)
	}

	// 檢查是否有可用日期
	if len(availableDays) == 0 {
		return fmt.Errorf("parking spot %d has no available days", spot.SpotID)
	}

	// 提取可用日期
	days := make([]string, len(availableDays))
	for i, day := range availableDays {
		days[i] = day.Day
	}

	// 檢查租用時間段內的每一天是否都在 available_days 中
	start := rent.StartTime
	end := rent.EndTime
	if start.After(end) {
		return fmt.Errorf("start_time cannot be later than end_time")
	}

	current := start
	for !current.After(end) {
		day := current.Weekday().String()
		isAvailable := false
		for _, availableDay := range days {
			if day == availableDay {
				isAvailable = true
				break
			}
		}
		if !isAvailable {
			return fmt.Errorf("parking spot %d is not available on %s", spot.SpotID, day)
		}
		current = current.Add(24 * time.Hour)
	}

	// 設置 actual_end_time 為 NULL
	rent.ActualEndTime = nil
	// 插入租用記錄
	if err := database.DB.Create(rent).Error; err != nil {
		log.Printf("Failed to rent parking spot: %v", err)
		return fmt.Errorf("failed to rent parking spot: %w", err)
	}

	// 更新車位狀態為使用中
	spot.Status = "in_use"
	if err := database.DB.Save(&spot).Error; err != nil {
		log.Printf("Failed to update parking spot status: %v", err)
		return fmt.Errorf("failed to update parking spot status: %w", err)
	}

	log.Printf("Successfully rented parking spot %d with rent ID %d", rent.SpotID, rent.RentID)
	return nil
}

// 查詢租用紀錄
func GetRentRecords() ([]models.Rent, error) {
	var rents []models.Rent
	// 查詢所有租用記錄
	if err := database.DB.Find(&rents).Error; err != nil {
		log.Printf("Failed to query rent records: %v", err)
		return nil, err
	}
	return rents, nil
}

// 取消租用
func CancelRent(id int) error {
	var rent models.Rent
	// 檢查租用是否存在
	if err := database.DB.First(&rent, id).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("Rent with ID %d not found", id)
			return nil
		}
		log.Printf("Failed to check rent: %v", err)
		return err
	}

	// 開始事務
	tx := database.DB.Begin()
	// 刪除租用記錄
	if err := tx.Delete(&rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to cancel rent: %v", err)
		return err
	}

	// 更新車位狀態
	var spot models.ParkingSpot
	if err := tx.Where("spot_id = ?", rent.SpotID).First(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to find parking spot: %v", err)
		return err
	}
	spot.Status = "idle"
	if err := tx.Save(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status: %v", err)
		return err
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction: %v", err)
		return err
	}
	return nil
}

// 離開付款
func LeaveAndPay(rentID int, actualEndTime time.Time) (float64, error) {
	var rent models.Rent
	var spot models.ParkingSpot

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred: %v", r)
		}
	}()

	// 查詢租用記錄和車位資訊
	if err := tx.First(&rent, rentID).Error; err != nil {
		log.Printf("Failed to query rent with ID %d: %v", rentID, err)
		tx.Rollback()
		return 0, fmt.Errorf("failed to query rent: %w", err)
	}
	if err := tx.Where("spot_id = ?", rent.SpotID).First(&spot).Error; err != nil {
		log.Printf("Failed to query parking spot with ID %d: %v", rent.SpotID, err)
		tx.Rollback()
		return 0, fmt.Errorf("failed to query parking spot: %w", err)
	}

	// 驗證 spot_id 一致性
	if spot.SpotID != rent.SpotID {
		tx.Rollback()
		log.Printf("Inconsistent spot ID: rent.SpotID=%d, spot.SpotID=%d", rent.SpotID, spot.SpotID)
		return 0, fmt.Errorf("inconsistent spot ID: expected %d, got %d", rent.SpotID, spot.SpotID)
	}

	// 檢查 actual_end_time 是否早於 start_time
	if actualEndTime.Before(rent.StartTime) {
		log.Printf("Actual end time %v is earlier than start time %v", actualEndTime, rent.StartTime)
		tx.Rollback()
		return 0, fmt.Errorf("actual end time cannot be earlier than start time")
	}

	// 計算費用
	duration := actualEndTime.Sub(rent.StartTime)
	minutes := int(duration.Minutes())

	overtimeMinutes := int(actualEndTime.Sub(rent.EndTime).Minutes())
	overtimeCost := 0.0
	if overtimeMinutes > 0 {
		overtimeHalfHours := math.Ceil(float64(overtimeMinutes) / 30.0)
		overtimeCost = overtimeHalfHours * 10.0 // 超時每半小時加收 10 元
	}

	baseCost := 0.0
	if minutes > 5 {
		halfHours := math.Ceil(float64(minutes) / 30.0)
		baseCost = halfHours * spot.PricePerHalfHour
	} else if minutes > 0 {
		// 至少收取半小時費用（5 分鐘內免費）
		baseCost = spot.PricePerHalfHour
	}

	totalCost := baseCost + overtimeCost
	if spot.DailyMaxPrice > 0 && totalCost > spot.DailyMaxPrice {
		totalCost = spot.DailyMaxPrice
	}

	// 更新租用記錄
	rent.ActualEndTime = &actualEndTime
	rent.TotalCost = totalCost
	if err := tx.Save(&rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update rent with ID %d: %v", rentID, err)
		return 0, fmt.Errorf("failed to update rent: %w", err)
	}

	// 更新車位狀態
	spot.Status = "idle"
	if err := tx.Save(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status for ID %d: %v", spot.SpotID, err)
		return 0, fmt.Errorf("failed to update parking spot status: %w", err)
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction for rent ID %d: %v", rentID, err)
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully processed leave and pay for rent ID %d with total cost %.2f", rentID, totalCost)
	return totalCost, nil
}

// 每月收費
func MonthlySettlement() error {
	var rents []models.Rent
	currentTime := time.Now()

	// 查詢未結算的租用記錄
	if err := database.DB.Where("actual_end_time IS NULL").Find(&rents).Error; err != nil {
		log.Printf("Failed to query unsettled rents: %v", err)
		return fmt.Errorf("failed to query unsettled rents: %w", err)
	}

	if len(rents) == 0 {
		log.Printf("No unsettled rents found for settlement at %v", currentTime)
		return nil
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred during monthly settlement: %v", r)
		}
	}()

	for i, rent := range rents {
		// 查詢車位價格
		var spot models.ParkingSpot
		if err := tx.Where("spot_id = ?", rent.SpotID).First(&spot).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to query parking spot with ID %d: %v", rent.SpotID, err)
			return fmt.Errorf("failed to query parking spot for rent %d: %w", rent.RentID, err)
		}

		// 驗證 spot_id 一致性
		if spot.SpotID != rent.SpotID {
			tx.Rollback()
			log.Printf("Inconsistent spot ID: rent.SpotID=%d, spot.SpotID=%d", rent.SpotID, spot.SpotID)
			return fmt.Errorf("inconsistent spot ID for rent %d: expected %d, got %d", rent.RentID, rent.SpotID, spot.SpotID)
		}

		// 計算費用
		actualEndTime := currentTime
		duration := actualEndTime.Sub(rent.StartTime)
		minutes := int(duration.Minutes())

		overtimeMinutes := int(actualEndTime.Sub(rent.EndTime).Minutes())
		overtimeCost := 0.0
		if overtimeMinutes > 0 {
			overtimeHalfHours := math.Ceil(float64(overtimeMinutes) / 30.0)
			overtimeCost = overtimeHalfHours * 10.0 // 超時每半小時加收 10 元
		} else if overtimeMinutes < 0 {
			log.Printf("Actual end time %v is earlier than end time %v for rent %d", actualEndTime, rent.EndTime, rent.RentID)
			tx.Rollback()
			return fmt.Errorf("actual end time cannot be earlier than end time for rent %d", rent.RentID)
		}

		baseCost := 0.0
		if minutes > 5 {
			halfHours := math.Ceil(float64(minutes) / 30.0)
			baseCost = halfHours * spot.PricePerHalfHour
		} else if minutes > 0 {
			// 至少收取半小時費用（5 分鐘內免費）
			baseCost = spot.PricePerHalfHour
		}

		totalCost := baseCost + overtimeCost
		if spot.DailyMaxPrice > 0 && totalCost > spot.DailyMaxPrice {
			totalCost = spot.DailyMaxPrice
		}

		// 更新租用記錄
		rents[i].ActualEndTime = &actualEndTime // 使用指標賦值
		rents[i].TotalCost = totalCost
		if err := tx.Save(&rents[i]).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to update rent with ID %d: %v", rent.RentID, err)
			return fmt.Errorf("failed to update rent %d: %w", rent.RentID, err)
		}

		// 更新車位狀態
		spot.Status = "idle"
		if err := tx.Save(&spot).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to update parking spot status for ID %d: %v", spot.SpotID, err)
			return fmt.Errorf("failed to update parking spot %d: %w", spot.SpotID, err)
		}
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction during monthly settlement: %v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully completed monthly settlement for %d rents at %v", len(rents), currentTime)
	return nil
}

// GetRentByID 查詢特定租賃記錄
func GetRentByID(id int) (*models.Rent, []string, error) {
	var rent models.Rent
	if err := database.DB.
		Preload("Member").
		Preload("ParkingSpot").
		Preload("ParkingSpot.Member").
		First(&rent, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Rent with ID %d not found", id)
			return nil, nil, nil
		}
		log.Printf("Failed to get rent by ID %d: %v", id, err)
		return nil, nil, fmt.Errorf("database error: %w", err)
	}

	// Fetch available days for the parking spot
	availableDays, err := FetchAvailableDays(rent.SpotID)
	if err != nil {
		log.Printf("Error fetching available days for spot %d: %v", rent.SpotID, err)
		// Continue with empty days to avoid failing the entire request
		availableDays = []string{}
	}

	return &rent, availableDays, nil
}
