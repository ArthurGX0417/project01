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

// CreateParkingLot 新增停車場並生成 spots
func CreateParkingLot(lot *models.ParkingLot) error {
	if err := database.DB.Create(lot).Error; err != nil {
		log.Printf("Failed to create parking lot: %v", err)
		return fmt.Errorf("failed to create parking lot: %w", err)
	}

	// 生成 total_spots 個 available spots
	for i := 0; i < lot.TotalSpots; i++ {
		spot := models.ParkingSpot{
			ParkingLotID: lot.ParkingLotID,
			Status:       "available",
		}
		if err := database.DB.Create(&spot).Error; err != nil {
			log.Printf("Failed to create spot for lot %d: %v", lot.ParkingLotID, err)
			return fmt.Errorf("failed to create spot for lot %d: %w", lot.ParkingLotID, err)
		}
	}

	log.Printf("Successfully created parking lot ID %d with %d spots", lot.ParkingLotID, lot.TotalSpots)
	return nil
}

// UpdateParkingLot 更新停車場資訊，並同步實際車位數量
func UpdateParkingLot(id int, updatedFields map[string]interface{}) (*models.ParkingLot, error) {
	var lot models.ParkingLot
	if err := database.DB.First(&lot, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("parking lot with ID %d not found", id)
		}
		return nil, fmt.Errorf("failed to find parking lot with ID %d: %w", id, err)
	}

	// 驗證並映射字段
	mappedFields := make(map[string]interface{})
	var newTotalSpots *int
	for key, value := range updatedFields {
		switch key {
		case "type":
			typeStr, ok := value.(string)
			if !ok || (typeStr != "flat" && typeStr != "mechanical") {
				return nil, fmt.Errorf("invalid type: must be 'flat' or 'mechanical'")
			}
			mappedFields["type"] = typeStr
		case "address":
			addressStr, ok := value.(string)
			if !ok || len(addressStr) > 100 {
				return nil, fmt.Errorf("invalid address: max length 100")
			}
			mappedFields["address"] = addressStr
		case "hourly_rate":
			var rate float64
			switch v := value.(type) {
			case float64:
				rate = v
			case int:
				rate = float64(v)
			default:
				return nil, fmt.Errorf("invalid hourly_rate type: must be number")
			}
			if rate < 0 {
				return nil, fmt.Errorf("invalid hourly_rate: must be >= 0")
			}
			mappedFields["hourly_rate"] = rate
		case "total_spots":
			spots, ok := value.(int)
			if !ok {
				return nil, fmt.Errorf("invalid total_spots type: expected int")
			}
			if spots < 0 {
				return nil, fmt.Errorf("invalid total_spots: must be >= 0")
			}
			mappedFields["total_spots"] = spots
			newTotalSpots = &spots
		case "longitude":
			lon, ok := value.(float64)
			if !ok || lon < -180 || lon > 180 {
				return nil, fmt.Errorf("invalid longitude: must be between -180 and 180")
			}
			mappedFields["longitude"] = lon
		case "latitude":
			lat, ok := value.(float64)
			if !ok || lat < -90 || lat > 90 {
				return nil, fmt.Errorf("invalid latitude: must be between -90 and 90")
			}
			mappedFields["latitude"] = lat
		default:
			return nil, fmt.Errorf("invalid field: %s", key)
		}
	}

	// 更新 parking_lot 表
	if err := database.DB.Model(&lot).Updates(mappedFields).Error; err != nil {
		return nil, fmt.Errorf("failed to update parking lot with ID %d: %w", id, err)
	}

	// === 處理 total_spots 變化：用「實際車位數」計算 diff ===
	if newTotalSpots != nil {
		targetSpots := *newTotalSpots

		// 1. 計算當前實際車位數（所有 status）
		var currentTotal int64
		if err := database.DB.Model(&models.ParkingSpot{}).
			Where("parking_lot_id = ?", id).
			Count(&currentTotal).Error; err != nil {
			log.Printf("Failed to count current spots for lot %d: %v", id, err)
			currentTotal = 0
		}

		// 2. 計算 occupied 數量（不能刪）
		var occupiedCount int64
		if err := database.DB.Model(&models.ParkingSpot{}).
			Where("parking_lot_id = ? AND status = ?", id, "occupied").
			Count(&occupiedCount).Error; err != nil {
			log.Printf("Failed to count occupied spots for lot %d: %v", id, err)
			occupiedCount = 0
		}

		// 3. 目標不能小於 occupied
		if targetSpots < int(occupiedCount) {
			return nil, fmt.Errorf("cannot reduce total_spots to %d: %d spots are occupied", targetSpots, occupiedCount)
		}

		// 4. 計算 diff
		diff := targetSpots - int(currentTotal)

		if diff > 0 {
			// 需要新增車位
			for i := 0; i < diff; i++ {
				spot := models.ParkingSpot{
					ParkingLotID: id,
					Status:       "available",
				}
				if err := database.DB.Create(&spot).Error; err != nil {
					log.Printf("Failed to create spot for lot %d: %v", id, err)
				}
			}
			log.Printf("Added %d new spots for parking lot %d (total: %d)", diff, id, targetSpots)
		} else if diff < 0 {
			// 需要刪除車位（只刪 available）
			deleteCount := -diff
			var spotsToDelete []models.ParkingSpot

			if err := database.DB.
				Where("parking_lot_id = ? AND status = ?", id, "available").
				Limit(deleteCount).
				Find(&spotsToDelete).Error; err != nil {
				log.Printf("Failed to query available spots for deletion in lot %d: %v", id, err)
			} else if len(spotsToDelete) > 0 {
				var spotIDs []int
				for _, s := range spotsToDelete {
					spotIDs = append(spotIDs, s.SpotID)
				}
				if err := database.DB.Delete(&models.ParkingSpot{}, spotIDs).Error; err != nil {
					log.Printf("Failed to delete spots: %v", err)
				} else {
					log.Printf("Deleted %d unused spots from parking lot %d (total: %d)", len(spotIDs), id, targetSpots)
				}
			}

			// 最終檢查（可選：若刪不夠可回報）
			var finalCount int64
			database.DB.Model(&models.ParkingSpot{}).
				Where("parking_lot_id = ?", id).
				Count(&finalCount)
			if int(finalCount) != targetSpots {
				log.Printf("Warning: expected %d spots, but got %d after update", targetSpots, finalCount)
			}
		}
	}

	log.Printf("Successfully updated parking lot with ID %d", id)
	return &lot, nil
}

// DeleteParkingLot 刪除停車場 (級聯刪除 spots 和 rents)
func DeleteParkingLot(id int) error {
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred during delete parking lot: %v", r)
		}
	}()

	var lot models.ParkingLot
	if err := tx.First(&lot, id).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("parking lot with ID %d not found", id)
		}
		return fmt.Errorf("failed to find parking lot with ID %d: %w", id, err)
	}

	// 刪除相關 rents (ON DELETE SET NULL in schema, but explicit delete for safety)
	if err := tx.Where("spot_id IN (SELECT spot_id FROM parking_spot WHERE parking_lot_id = ?)", id).Delete(&models.Rent{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete rents for lot %d: %w", id, err)
	}

	// 刪除 spots (ON DELETE CASCADE in schema)
	if err := tx.Where("parking_lot_id = ?", id).Delete(&models.ParkingSpot{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete spots for lot %d: %w", id, err)
	}

	// 刪除 lot
	if err := tx.Delete(&lot).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete parking lot with ID %d: %w", id, err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction for delete lot %d: %w", id, err)
	}

	log.Printf("Successfully deleted parking lot with ID %d", id)
	return nil
}

// GetAllParkingLots 查詢所有停車場（限 admin）
func GetAllParkingLots() ([]models.ParkingLot, error) {
	var parkingLots []models.ParkingLot
	if err := database.DB.Preload("ParkingSpots").Find(&parkingLots).Error; err != nil {
		log.Printf("Failed to query all parking lots: %v", err)
		return nil, fmt.Errorf("failed to query all parking lots: %w", err)
	}

	for i := range parkingLots {
		var occupiedSpots int64
		database.DB.Model(&models.Rent{}).
			Where("parking_lot_id = ? AND end_time IS NULL", parkingLots[i].ParkingLotID).
			Count(&occupiedSpots)
		parkingLots[i].RemainingSpots = parkingLots[i].TotalSpots - int(occupiedSpots)
	}

	log.Printf("Successfully retrieved %d parking lots", len(parkingLots))
	return parkingLots, nil
}
