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
		return fmt.Errorf("failed to verify member: %w", err)
	}

	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic recovered in LeaveParkingSpot: %v", r)
		}
	}()

	var spots []models.ParkingSpot
	if err := tx.Where("parking_lot_id = ? AND status = ?", parkingLotID, "available").Find(&spots).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("query available spots failed: %w", err)
	}
	if len(spots) == 0 {
		tx.Rollback()
		return fmt.Errorf("no available parking spots in lot %d", parkingLotID)
	}

	spot := spots[0]
	spot.Status = "occupied"

	// 關鍵：SpotID 是 *int，所以要取址
	spotIDPtr := &spot.SpotID

	rent := &models.Rent{
		LicensePlate: licensePlate,
		StartTime:    startTime,
		SpotID:       spotIDPtr,
		Status:       "pending",
	}

	if err := tx.Create(rent).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("create rent failed: %w", err)
	}

	if err := tx.Save(&spot).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("update spot status failed: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit transaction failed: %w", err)
	}

	log.Printf("Successfully entered: license_plate=%s, start_time=%s, spot_id=%d",
		licensePlate, startTime.Format(time.RFC3339), spot.SpotID)
	return nil
}

// LeaveParkingSpot 出場記錄並計算費用
func LeaveParkingSpot(licensePlate string, endTime time.Time) error {
	var rent models.Rent
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 查出 pending 的租借 + 預載車位與停車場
	if err := tx.Preload("Spot.ParkingLot").
		Where("license_plate = ? AND status = ?", licensePlate, "pending").
		Order("start_time DESC").
		First(&rent).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("no pending rent for %s", licensePlate)
		}
		return fmt.Errorf("query rent failed: %w", err)
	}

	// 檢查停車場是否正確載入
	if rent.Spot.ParkingLot.ParkingLotID == 0 {
		tx.Rollback()
		return fmt.Errorf("parking lot not found for spot_id=%v", rent.SpotID)
	}

	// 計算費用
	totalCost, err := CalculateRentCost(rent.StartTime, endTime, rent.Spot.ParkingLot)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("calculate cost failed: %w", err)
	}

	// 更新 rent（TotalCost 是 *float64）
	rent.TotalCost = &totalCost
	rent.EndTime = &endTime
	rent.Status = "completed"

	if err := tx.Save(&rent).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("save rent failed: %w", err)
	}

	// 釋放車位
	rent.Spot.Status = "available"
	if err := tx.Save(&rent.Spot).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("release spot failed: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("commit failed: %w", err)
	}

	log.Printf("Successfully left: license_plate=%s, start_time=%s, cost=%.2f",
		licensePlate, rent.StartTime.Format(time.RFC3339), totalCost)
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
		Preload("Spot.ParkingLot").
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
