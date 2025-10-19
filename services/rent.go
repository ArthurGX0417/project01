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

// CalculateRentCost 根據進場和出場時間計算租賃費用，基於 parking_lot 的 hourly_rate，向上取整
func CalculateRentCost(startTime time.Time, endTime time.Time, parkingLot models.ParkingLot) (float64, error) {
	if endTime.Before(startTime) {
		log.Printf("end_time %v is before start_time %v", endTime, startTime)
		return 0, fmt.Errorf("end_time %v cannot be earlier than start_time %v", endTime, startTime)
	}

	if parkingLot.HourlyRate <= 0 {
		return 0, fmt.Errorf("invalid hourly_rate for parking_lot_id %d: HourlyRate=%.2f", parkingLot.ParkingLotID, parkingLot.HourlyRate)
	}

	durationMinutes := endTime.Sub(startTime).Minutes()
	durationHours := math.Ceil(durationMinutes / 60.0)

	totalCost := durationHours * parkingLot.HourlyRate
	return totalCost, nil
}

// EnterParkingSpot 進場記錄車牌和進場時間
func EnterParkingSpot(licensePlate string, parkingLotID int, startTime time.Time) error {
	var member models.Member
	if err := database.DB.Where("license_plate = ?", licensePlate).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("member with license_plate %s not found", licensePlate)
		}
		log.Printf("Failed to verify member: license_plate=%s, error=%v", licensePlate, err)
		return fmt.Errorf("failed to verify member: %w", err)
	}

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred during enter parking spot: license_plate=%s, error=%v", licensePlate, r)
		}
	}()

	var parkingSpots []models.ParkingSpot
	if err := tx.Where("parking_lot_id = ? AND status = ?", parkingLotID, "available").Find(&parkingSpots).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to query available parking spots: error=%v", err)
		return fmt.Errorf("failed to query available parking spots: %w", err)
	}
	if len(parkingSpots) == 0 {
		tx.Rollback()
		return fmt.Errorf("no available parking spots in lot %d", parkingLotID)
	}

	spot := parkingSpots[0]
	spot.Status = "occupied"

	rent := &models.Rent{
		MemberID:     member.MemberID,
		SpotID:       spot.SpotID,
		LicensePlate: licensePlate,
		StartTime:    startTime,
		Status:       "pending",
	}

	if err := tx.Create(rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to create rent record: license_plate=%s, error=%v", licensePlate, err)
		return fmt.Errorf("failed to create rent record: %w", err)
	}

	if err := tx.Save(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", spot.SpotID, err)
		return fmt.Errorf("failed to update parking spot status: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction: license_plate=%s, error=%v", licensePlate, err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully entered parking spot: license_plate=%s, rent_id=%d, spot_id=%d", licensePlate, rent.RentID, spot.SpotID)
	return nil
}

// LeaveParkingSpot 出場記錄並計算費用
func LeaveParkingSpot(licensePlate string, endTime time.Time) error {
	var rent models.Rent
	var spot models.ParkingSpot

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred: license_plate=%s, error=%v", licensePlate, r)
		}
	}()

	if err := tx.Where("license_plate = ? AND status = ?", licensePlate, "pending").
		Order("start_time DESC").First(&rent).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no pending rent found for license_plate %s", licensePlate)
		}
		log.Printf("Failed to query rent: license_plate=%s, error=%v", licensePlate, err)
		return fmt.Errorf("failed to query rent: %w", err)
	}

	if err := tx.Preload("ParkingSpot").First(&rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to preload parking spot: rent_id=%d, error=%v", rent.RentID, err)
		return fmt.Errorf("failed to preload parking spot: %w", err)
	}
	spot = rent.ParkingSpot

	totalCost, err := CalculateRentCost(rent.StartTime, endTime, spot.ParkingLot)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to calculate rent cost: rent_id=%d, error=%v", rent.RentID, err)
		return fmt.Errorf("failed to calculate rent cost: %w", err)
	}
	rent.TotalCost = totalCost
	rent.EndTime = &endTime // 將 time.Time 轉為 *time.Time
	rent.Status = "completed"

	if err := tx.Save(&rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update rent: rent_id=%d, error=%v", rent.RentID, err)
		return fmt.Errorf("failed to update rent: %w", err)
	}

	spot.Status = "available"
	if err := tx.Save(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", spot.SpotID, err)
		return fmt.Errorf("failed to update parking spot status: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction: rent_id=%d, error=%v", rent.RentID, err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully left parking spot: license_plate=%s, rent_id=%d, total_cost=%.2f", licensePlate, rent.RentID, totalCost)
	return nil
}

// GetRentRecordsByLicensePlate 查詢車主的所有租用紀錄
func GetRentRecordsByLicensePlate(licensePlate string, requestingLicensePlate string) ([]models.Rent, error) {
	var member models.Member
	if err := database.DB.Where("license_plate = ?", requestingLicensePlate).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("member with license_plate %s not found", requestingLicensePlate)
		}
		log.Printf("Failed to verify requesting member: license_plate=%s, error=%v", requestingLicensePlate, err)
		return nil, fmt.Errorf("failed to verify requesting member: %w", err)
	}

	if licensePlate != requestingLicensePlate {
		log.Printf("Unauthorized access: requesting_license_plate=%s, target_license_plate=%s", requestingLicensePlate, licensePlate)
		return nil, fmt.Errorf("unauthorized access to rent records")
	}

	var rents []models.Rent
	if err := database.DB.
		Preload("ParkingSpot.ParkingLot").
		Where("license_plate = ?", licensePlate).
		Find(&rents).Error; err != nil {
		log.Printf("Failed to query rent records for license_plate %s: error=%v", licensePlate, err)
		return nil, fmt.Errorf("failed to query rent records: %w", err)
	}

	log.Printf("Successfully fetched %d rent records for license_plate %s", len(rents), licensePlate)
	return rents, nil
}

// GetTotalCostByLicensePlate 計算車主總停車費用
func GetTotalCostByLicensePlate(licensePlate string) (float64, error) {
	var totalCost float64
	err := database.DB.Model(&models.Rent{}).
		Where("license_plate = ? AND status = ?", licensePlate, "completed").
		Select("COALESCE(SUM(total_cost), 0)").Scan(&totalCost).Error
	if err != nil {
		log.Printf("Failed to calculate total cost for license_plate %s: error=%v", licensePlate, err)
		return 0, fmt.Errorf("failed to calculate total cost: %w", err)
	}
	log.Printf("Successfully calculated total cost %.2f for license_plate %s", totalCost, licensePlate)
	return totalCost, nil
}

// CheckParkingAvailability 查詢特定停車場可用位子
func CheckParkingAvailability(parkingLotID int) (int64, error) {
	var totalSpots, occupiedSpots int64

	if parkingLotID > 0 {
		// 指定場：count該lot spots
		if err := database.DB.Model(&models.ParkingLot{}).Where("parking_lot_id = ?", parkingLotID).Select("total_spots").Scan(&totalSpots).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return 0, fmt.Errorf("parking lot %d not found", parkingLotID)
			}
			log.Printf("Failed to get total spots for lot %d: %v", parkingLotID, err)
			return 0, fmt.Errorf("failed to get total spots: %w", err)
		}
		if err := database.DB.Model(&models.ParkingSpot{}).Where("parking_lot_id = ? AND status = ?", parkingLotID, "occupied").Count(&occupiedSpots).Error; err != nil {
			log.Printf("Failed to count occupied parking spots for lot %d: %v", parkingLotID, err)
			return 0, fmt.Errorf("failed to count occupied parking spots: %w", err)
		}
	} else {
		// 全域：sum all lots total_spots
		if err := database.DB.Model(&models.ParkingLot{}).Select("SUM(total_spots)").Scan(&totalSpots).Error; err != nil {
			log.Printf("Failed to sum total parking spots: %v", err)
			return 0, fmt.Errorf("failed to sum total parking spots: %w", err)
		}
		if err := database.DB.Model(&models.ParkingSpot{}).Where("status = ?", "occupied").Count(&occupiedSpots).Error; err != nil {
			log.Printf("Failed to count occupied parking spots: %v", err)
			return 0, fmt.Errorf("failed to count occupied parking spots: %w", err)
		}
	}

	availableSpots := totalSpots - occupiedSpots
	log.Printf("Available spots: %d (parking_lot_id=%d)", availableSpots, parkingLotID)
	return availableSpots, nil
}

// GenerateParkingNotification 生成停車通知
func GenerateParkingNotification(rentID int) (map[string]interface{}, error) {
	var rent models.Rent
	if err := database.DB.Preload("ParkingSpot.ParkingLot").First(&rent, rentID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("rent with ID %d not found", rentID)
		}
		return nil, fmt.Errorf("failed to get rent: %w", err)
	}

	notification := map[string]interface{}{
		"rent_id":       rent.RentID,
		"license_plate": rent.LicensePlate,
		"start_time":    rent.StartTime.Format("2006-01-02T15:04:05+08:00"),
	}
	if rent.EndTime != nil {
		notification["end_time"] = rent.EndTime.Format("2006-01-02T15:04:05+08:00")
		notification["total_cost"] = rent.TotalCost
	} else {
		notification["end_time"] = "N/A"
		notification["total_cost"] = 0.0
	}

	log.Printf("Generated notification for rent_id %d: %+v", rentID, notification)
	return notification, nil
}

// GetCurrentlyRentedSpots 查詢當前租用的車位
func GetCurrentlyRentedSpots(licensePlate string) ([]models.Rent, error) {
	var rents []models.Rent
	if err := database.DB.Where("license_plate = ? AND status = ?", licensePlate, "pending").Find(&rents).Error; err != nil {
		log.Printf("Failed to get currently rented spots for license_plate %s: %v", licensePlate, err)
		return nil, fmt.Errorf("failed to get currently rented spots: %w", err)
	}
	log.Printf("Successfully retrieved %d currently rented spots for license_plate %s", len(rents), licensePlate)
	return rents, nil
}

// GetRentByID 查詢特定租賃記錄
func GetRentByID(rentID int) (*models.Rent, error) {
	var rent models.Rent
	if err := database.DB.Where("rent_id = ?", rentID).First(&rent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Rent with ID %d not found", rentID)
			return nil, nil
		}
		log.Printf("Failed to get rent by ID %d: %v", rentID, err)
		return nil, fmt.Errorf("failed to get rent by ID %d: %w", rentID, err)
	}
	log.Printf("Successfully retrieved rent with ID %d", rentID)
	return &rent, nil
}
