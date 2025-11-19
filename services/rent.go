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
	// 驗證車牌是否存在於 vehicle 表（未來改成查 vehicle）
	var vehicle models.Vehicle
	if err := database.DB.Where("license_plate = ?", licensePlate).First(&vehicle).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("車牌 %s 未註冊", licensePlate)
		}
		return fmt.Errorf("查詢車輛失敗: %w", err)
	}

	rent := &models.Rent{
		LicensePlate: licensePlate,
		ParkingLotID: parkingLotID,
		StartTime:    startTime,
	}

	if err := database.DB.Create(rent).Error; err != nil {
		return fmt.Errorf("進場失敗: %w", err)
	}

	log.Printf("進場成功：%s 於 %s 進入停車場 %d", licensePlate, startTime.Format("15:04:05"), parkingLotID)
	return nil
}

// LeaveParkingSpot 出場記錄並計算費用
func LeaveParkingSpot(licensePlate string, endTime time.Time) error {
	var rent models.Rent

	if err := database.DB.
		Preload("ParkingLot").
		Where("license_plate = ? AND end_time IS NULL", licensePlate). // 改這裡！
		Order("start_time DESC").
		First(&rent).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("車牌 %s 目前沒有停車中的紀錄", licensePlate)
		}
		return fmt.Errorf("查詢停車紀錄失敗: %w", err)
	}

	totalCost, err := CalculateRentCost(rent.StartTime, endTime, rent.ParkingLot)
	if err != nil {
		return fmt.Errorf("計算費用失敗: %w", err)
	}

	rent.EndTime = &endTime
	rent.TotalCost = &totalCost

	if err := database.DB.Save(&rent).Error; err != nil {
		return fmt.Errorf("離場更新失敗: %w", err)
	}

	log.Printf("離場成功：%s 停車 %.2f 小時，費用 %.2f 元", licensePlate, endTime.Sub(rent.StartTime).Hours(), totalCost)
	return nil
}

// GetRentRecordsByLicensePlate 查詢車主的所有租用紀錄
func GetRentRecordsByLicensePlate(licensePlate string, requestingLicensePlate string) ([]models.Rent, error) {
	if licensePlate != requestingLicensePlate {
		return nil, fmt.Errorf("無權查看他人紀錄")
	}

	var rents []models.Rent
	if err := database.DB.
		Preload("ParkingLot").
		Where("license_plate = ?", licensePlate).
		Order("start_time DESC").
		Find(&rents).Error; err != nil {
		return nil, err
	}
	return rents, nil
}

// GetTotalCostByLicensePlate 計算車主總停車費用
func GetTotalCostByLicensePlate(licensePlate string) (float64, error) {
	var totalCost float64
	err := database.DB.Model(&models.Rent{}).
		Where("license_plate = ? AND end_time IS NOT NULL", licensePlate). // 改這裡！
		Select("COALESCE(SUM(total_cost), 0)").
		Scan(&totalCost).Error
	if err != nil {
		return 0, err
	}
	return totalCost, nil
}

// CheckParkingAvailability 查詢特定停車場可用位子
func CheckParkingAvailability(parkingLotID int) (int64, error) {
	var totalSpots, parkingCount int64

	if err := database.DB.Model(&models.ParkingLot{}).
		Where(" parking_lot_id = ?", parkingLotID).
		Pluck("total_spots", &totalSpots).Error; err != nil {
		return 0, err
	}

	if err := database.DB.Model(&models.Rent{}).
		Where("parking_lot_id = ? AND end_time IS NULL", parkingLotID). // 改這裡！
		Count(&parkingCount).Error; err != nil {
		return 0, err
	}

	available := totalSpots - parkingCount
	if available < 0 {
		available = 0
	}
	log.Printf("停車場 %d：總車位 %d，停車中 %d，剩餘 %d", parkingLotID, totalSpots, parkingCount, available)
	return available, nil
}

// GetCurrentlyRentedSpots 查詢當前租用的車位
func GetCurrentlyRentedSpots(licensePlate string) ([]models.Rent, error) {
	var rents []models.Rent
	if err := database.DB.
		Where("license_plate = ? AND end_time IS NULL", licensePlate). // 改這裡！
		Find(&rents).Error; err != nil {
		return nil, fmt.Errorf("查詢停車中紀錄失敗: %w", err)
	}
	return rents, nil
}
