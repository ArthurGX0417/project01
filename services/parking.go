package services

import (
	"errors"
	"fmt"
	"log"
	"project01/database"
	"project01/models"

	"gorm.io/gorm"
)

// GetAvailableParkingLots 查詢附近有剩餘車位的停車場（用 pending rent 計算！）
func GetAvailableParkingLots(latitude, longitude, radius float64) ([]models.ParkingLot, error) {
	var lots []models.ParkingLot

	if radius <= 0 {
		radius = 3.0
	}
	if radius > 50 {
		radius = 50.0
	}

	// 哈弗辛公式計算距離
	distanceSQL := `
        6371 * acos(
            cos(radians(?)) * cos(radians(latitude)) * 
            cos(radians(longitude) - radians(?)) + 
            sin(radians(?)) * sin(radians(latitude))
        )
    `

	query := database.DB.Where(distanceSQL+" <= ?", latitude, longitude, latitude, radius)
	if err := query.Find(&lots).Error; err != nil {
		log.Printf("Failed to query parking lots: %v", err)
		return nil, fmt.Errorf("failed to query parking lots: %w", err)
	}

	// 計算每個停車場剩餘車位（用 pending rent 數量）
	filteredLots := []models.ParkingLot{}
	for _, lot := range lots {
		var pendingCount int64
		if err := database.DB.Model(&models.Rent{}).
			Where("parking_lot_id = ? AND status = ?", lot.ParkingLotID, "pending").
			Count(&pendingCount).Error; err != nil {
			log.Printf("Failed to count pending rents for lot %d: %v", lot.ParkingLotID, err)
			continue
		}

		remaining := lot.TotalSpots - int(pendingCount)
		if remaining > 0 {
			lot.RemainingSpots = remaining
			filteredLots = append(filteredLots, lot)
		}
	}

	log.Printf("Successfully retrieved %d available parking lots within %.1f km", len(filteredLots), radius)
	return filteredLots, nil
}

// GetParkingLotByID 查詢單一停車場（含即時剩餘車位）
func GetParkingLotByID(id int) (*models.ParkingLot, error) {
	var lot models.ParkingLot
	if err := database.DB.First(&lot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get parking lot %d: %w", id, err)
	}

	// 計算即時剩餘車位
	var pendingCount int64
	if err := database.DB.Model(&models.Rent{}).
		Where("parking_lot_id = ? AND status = ?", id, "pending").
		Count(&pendingCount).Error; err != nil {
		lot.RemainingSpots = 0
	} else {
		lot.RemainingSpots = lot.TotalSpots - int(pendingCount)
		if lot.RemainingSpots < 0 {
			lot.RemainingSpots = 0
		}
	}

	return &lot, nil
}

// CreateParkingLot 只建立停車場，不建立任何 spot
func CreateParkingLot(lot *models.ParkingLot) error {
	if err := database.DB.Create(lot).Error; err != nil {
		return fmt.Errorf("failed to create parking lot: %w", err)
	}
	log.Printf("Successfully created parking lot ID %d with %d virtual spots", lot.ParkingLotID, lot.TotalSpots)
	return nil
}

// UpdateParkingLot 更新停車場資訊（total_spots 直接改，不動任何實體車位）
func UpdateParkingLot(id int, updatedFields map[string]interface{}) (*models.ParkingLot, error) {
	var lot models.ParkingLot
	if err := database.DB.First(&lot, id).Error; err != nil {
		return nil, fmt.Errorf("parking lot %d not found: %w", id, err)
	}

	// 直接更新允許的欄位
	if err := database.DB.Model(&lot).Updates(updatedFields).Error; err != nil {
		return nil, fmt.Errorf("failed to update parking lot %d: %w", id, err)
	}

	// 重新計算剩餘車位
	var pending int64
	database.DB.Model(&models.Rent{}).
		Where("parking_lot_id = ? AND status = ?", id, "pending").
		Count(&pending)
	lot.RemainingSpots = lot.TotalSpots - int(pending)
	if lot.RemainingSpots < 0 {
		lot.RemainingSpots = 0
	}

	log.Printf("Successfully updated parking lot %d, new total spots: %d", id, lot.TotalSpots)
	return &lot, nil
}

// DeleteParkingLot 刪除停車場（連同所有 rent 紀錄一起刪）
func DeleteParkingLot(id int) error {
	tx := database.DB.Begin()

	// 刪除該停車場的所有 rent 紀錄
	if err := tx.Where("parking_lot_id = ?", id).Delete(&models.Rent{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete rents for lot %d: %w", id, err)
	}

	// 刪除停車場
	if err := tx.Delete(&models.ParkingLot{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete parking lot %d: %w", id, err)
	}

	tx.Commit()
	log.Printf("Successfully deleted parking lot %d and all its rent records", id)
	return nil
}

// GetAllParkingLots 取得所有停車場（含即時剩餘車位）
func GetAllParkingLots() ([]models.ParkingLot, error) {
	var lots []models.ParkingLot
	if err := database.DB.Find(&lots).Error; err != nil {
		return nil, fmt.Errorf("failed to query parking lots: %w", err)
	}

	for i := range lots {
		var pending int64
		database.DB.Model(&models.Rent{}).
			Where("parking_lot_id = ? AND status = ?", lots[i].ParkingLotID, "pending").
			Count(&pending)
		lots[i].RemainingSpots = lots[i].TotalSpots - int(pending)
		if lots[i].RemainingSpots < 0 {
			lots[i].RemainingSpots = 0
		}
	}

	return lots, nil
}
