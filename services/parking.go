package services

import (
	"errors"
	"fmt"
	"log"
	"project01/database"
	"project01/models"

	"gorm.io/gorm"
)

// GetAvailableParkingLots 查詢可用停車場，基於經緯度和半徑（即時可用，無日期）
func GetAvailableParkingLots(latitude, longitude, radius float64) ([]models.ParkingLot, error) {
	var lots []models.ParkingLot

	if radius <= 0 {
		radius = 3.0
	}
	if radius > 50 {
		radius = 50.0
	}

	distanceSQL := `
        6371 * acos(
            cos(radians(?)) * cos(radians(latitude)) * 
            cos(radians(longitude) - radians(?)) + 
            sin(radians(?)) * sin(radians(latitude))
        )
    `

	// 查詢在半徑內的停車場
	query := database.DB.Where(distanceSQL+" <= ?", latitude, longitude, latitude, radius)
	if err := query.Find(&lots).Error; err != nil {
		log.Printf("Failed to query parking lots: %v", err)
		return nil, fmt.Errorf("failed to query parking lots: %w", err)
	}

	// 過濾並計算每個停車場的剩餘位子
	filteredLots := []models.ParkingLot{}
	for _, lot := range lots {
		var availableCount int64
		if err := database.DB.Model(&models.ParkingSpot{}).
			Where("parking_lot_id = ? AND status = ?", lot.ParkingLotID, "available").
			Count(&availableCount).Error; err != nil {
			log.Printf("Failed to count available spots for lot ID %d: %v", lot.ParkingLotID, err)
			continue // 錯誤時跳過
		}
		remaining := int(availableCount)
		if remaining > 0 { // 只保留剩餘 > 0 的
			lot.RemainingSpots = remaining
			filteredLots = append(filteredLots, lot)
		}
	}

	if len(filteredLots) == 0 {
		log.Printf("No available parking lots found within %f km", radius)
	}

	log.Printf("Successfully retrieved %d available parking lots within %f km", len(filteredLots), radius)
	return filteredLots, nil
}

// GetParkingLotByID 查詢特定停車場詳情，包括剩餘位子數量
func GetParkingLotByID(id int) (*models.ParkingLot, error) { // 改返回 *models.ParkingLot（lot 級）
	var lot models.ParkingLot
	if err := database.DB.First(&lot, id).Error; err != nil { // 直接查 parking_lot
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking lot with ID %d not found", id)
			return nil, nil
		}
		log.Printf("Failed to get parking lot by ID %d: %v", id, err)
		return nil, fmt.Errorf("failed to get parking lot by ID %d: %w", id, err)
	}

	// 計算剩餘位子數量
	var availableCount int64
	if err := database.DB.Model(&models.ParkingSpot{}).
		Where("parking_lot_id = ? AND status = ?", lot.ParkingLotID, "available").
		Count(&availableCount).Error; err != nil {
		log.Printf("Failed to count available spots for lot ID %d: %v", lot.ParkingLotID, err)
		lot.RemainingSpots = 0
	} else {
		lot.RemainingSpots = max(0, int(availableCount)) // 確保不負
	}

	log.Printf("Successfully retrieved parking lot with ID %d, remaining spots: %d", id, lot.RemainingSpots)
	return &lot, nil
}

// 輔助函數，確保剩餘數不為負
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
