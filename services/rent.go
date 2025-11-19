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

	// 直接建立 rent，不碰任何 parking_spot
	rent := &models.Rent{
		LicensePlate: licensePlate,
		ParkingLotID: parkingLotID, // 關鍵！記住停在哪個停車場
		StartTime:    startTime,
		Status:       "pending",
	}

	if err := database.DB.Create(rent).Error; err != nil {
		return fmt.Errorf("create rent failed: %w", err)
	}

	log.Printf("進場成功：%s 於 %s 進入停車場 %d（僅開單，不佔車位）",
		licensePlate, startTime.Format("15:04:05"), parkingLotID)
	return nil
}

// LeaveParkingSpot 出場記錄並計算費用
func LeaveParkingSpot(licensePlate string, endTime time.Time) error {
	var rent models.Rent

	// 直接 Preload ParkingLot（因為我們現在有 parking_lot_id 欄位）
	if err := database.DB.
		Preload("ParkingLot"). // 這裡改對了！
		Where("license_plate = ? AND status = ?", licensePlate, "pending").
		Order("start_time DESC").
		First(&rent).Error; err != nil {

		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("此車牌目前沒有未結帳的停車紀錄：%s", licensePlate)
		}
		return fmt.Errorf("查詢停車紀錄失敗: %w", err)
	}

	// 計算費用（直接用 rent.ParkingLot.HourlyRate）
	totalCost, err := CalculateRentCost(rent.StartTime, endTime, rent.ParkingLot)
	if err != nil {
		return fmt.Errorf("計算費用失敗: %w", err)
	}

	// 更新這筆 rent
	rent.EndTime = &endTime
	rent.TotalCost = &totalCost
	rent.Status = "completed"

	if err := database.DB.Save(&rent).Error; err != nil {
		return fmt.Errorf("更新離場資料失敗: %w", err)
	}

	log.Printf("離場成功：%s 停車 %.2f 小時，費用 %.2f 元",
		licensePlate, endTime.Sub(rent.StartTime).Hours(), totalCost)

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
		Preload("ParkingLot").
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
	var totalSpots int64
	var pendingCount int64

	// 1. 取得該停車場總車位數
	if err := database.DB.Model(&models.ParkingLot{}).
		Where("parking_lot_id = ?", parkingLotID).
		Pluck("total_spots", &totalSpots).Error; err != nil {
		return 0, fmt.Errorf("failed to get total spots: %w", err)
	}

	// 2. 計算該停車場目前 pending 的車（就是正在停的車）
	if err := database.DB.Model(&models.Rent{}).
		Where("parking_lot_id = ? AND status = ?", parkingLotID, "pending").
		Count(&pendingCount).Error; err != nil {
		return 0, fmt.Errorf("failed to count pending rents: %w", err)
	}

	available := totalSpots - pendingCount
	log.Printf("停車場 %d：總車位 %d，進行中 %d，剩餘 %d", parkingLotID, totalSpots, pendingCount, available)
	return available, nil
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
