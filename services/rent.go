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
	var vehicle models.Vehicle
	if err := database.DB.Where("license_plate = ?", licensePlate).First(&vehicle).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("vehicle not registered: license_plate=%s", licensePlate)
		}
		return fmt.Errorf("failed to query vehicle: %w", err)
	}

	rent := &models.Rent{
		LicensePlate: licensePlate,
		ParkingLotID: parkingLotID,
		StartTime:    startTime,
	}

	if err := database.DB.Create(rent).Error; err != nil {
		return fmt.Errorf("entry failed: %w", err)
	}

	log.Printf("ENTRY_SUCCESS | license_plate=%s parking_lot_id=%d entry_time=%s",
		licensePlate, parkingLotID, startTime.Format(time.RFC3339))

	return nil
}

// LeaveParkingSpot 出場記錄並計算費用
func LeaveParkingSpot(licensePlate string, endTime time.Time) (*models.Rent, error) {
	var rent models.Rent

	if err := database.DB.
		Preload("ParkingLot").
		Where("license_plate = ? AND end_time IS NULL", licensePlate).
		Order("start_time DESC").
		First(&rent).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("no active parking record found: license_plate=%s", licensePlate)
		}
		return nil, fmt.Errorf("failed to query active parking record: %w", err)
	}

	totalCost, err := CalculateRentCost(rent.StartTime, endTime, rent.ParkingLot)
	if err != nil {
		return nil, fmt.Errorf("fee calculation failed: %w", err)
	}

	rent.EndTime = &endTime
	rent.TotalCost = &totalCost

	if err := database.DB.Save(&rent).Error; err != nil {
		return nil, fmt.Errorf("exit update failed: %w", err)
	}

	duration := endTime.Sub(rent.StartTime).Hours()
	log.Printf("EXIT_SUCCESS | license_plate=%s parking_lot_id=%d duration_hours=%.2f cost=%.2f exit_time=%s",
		licensePlate, rent.ParkingLotID, duration, totalCost, endTime.Format(time.RFC3339))

	return &rent, nil // 關鍵！回傳完整 rent 紀錄
}

// GetRentRecordsByMemberID 查詢會員的所有租用紀錄
func GetRentRecordsByMemberID(memberID int) ([]models.Rent, error) {
	var rents []models.Rent

	// 先查這個 member 的所有車牌
	var vehiclePlates []string
	if err := database.DB.
		Model(&models.Vehicle{}).
		Where("member_id = ?", memberID).
		Pluck("license_plate", &vehiclePlates).Error; err != nil {
		return nil, fmt.Errorf("failed to get vehicles for member %d: %w", memberID, err)
	}

	if len(vehiclePlates) == 0 {
		return []models.Rent{}, nil // 沒車就回空陣列，不算錯誤
	}

	// 再查這些車牌的所有租賃紀錄
	if err := database.DB.
		Preload("ParkingLot").
		Where("license_plate IN ?", vehiclePlates).
		Order("start_time DESC").
		Find(&rents).Error; err != nil {
		return nil, fmt.Errorf("failed to fetch rent records: %w", err)
	}

	return rents, nil
}

// GetTotalCostByLicensePlate 計算車主總停車費用
func GetTotalCostByLicensePlate(licensePlate string) (float64, error) {
	var totalCost float64
	err := database.DB.Model(&models.Rent{}).
		Where("license_plate = ? AND end_time IS NOT NULL", licensePlate).
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&totalCost).Error
	if err != nil {
		return 0, fmt.Errorf("failed to calculate total cost: %w", err)
	}
	return totalCost, nil
}

// CheckParkingAvailability 查詢特定停車場可用位子
func CheckParkingAvailability(parkingLotID int) (int64, error) {
	var totalSpots, parkingCount int64

	if err := database.DB.Model(&models.ParkingLot{}).
		Where("parking_lot_id = ?", parkingLotID).
		Pluck("total_spots", &totalSpots).Error; err != nil {
		return 0, fmt.Errorf("failed to get total spots: %w", err)
	}

	if err := database.DB.Model(&models.Rent{}).
		Where("parking_lot_id = ? AND end_time IS NULL", parkingLotID).
		Count(&parkingCount).Error; err != nil {
		return 0, fmt.Errorf("failed to count parked vehicles: %w", err)
	}

	available := totalSpots - parkingCount
	if available < 0 {
		available = 0
	}

	log.Printf("AVAILABILITY | parking_lot_id=%d total_spots=%d occupied=%d available=%d",
		parkingLotID, totalSpots, parkingCount, available)

	return available, nil
}

// GetCurrentlyRentedSpotsByMemberID 查詢當前租用的車位（正在停中的車）
func GetCurrentlyRentedSpotsByMemberID(memberID int) ([]models.Rent, error) {
	var rents []models.Rent

	var vehiclePlates []string
	if err := database.DB.
		Model(&models.Vehicle{}).
		Where("member_id = ?", memberID).
		Pluck("license_plate", &vehiclePlates).Error; err != nil {
		return nil, fmt.Errorf("failed to get vehicles: %w", err)
	}

	if len(vehiclePlates) == 0 {
		return []models.Rent{}, nil
	}

	if err := database.DB.
		Where("license_plate IN ? AND end_time IS NULL", vehiclePlates).
		Find(&rents).Error; err != nil {
		return nil, fmt.Errorf("failed to query active records: %w", err)
	}

	log.Printf("ACTIVE_PARKING | member_id=%d active_records=%d", memberID, len(rents))
	return rents, nil
}
