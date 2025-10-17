package services

import (
	"errors"
	"fmt"
	"log"
	"project01/database"
	"project01/models"
	"time"

	"gorm.io/gorm"
)

// GetAvailableParkingLots 查詢可用停車場，基於日期和經緯度
func GetAvailableParkingLots(date string, latitude, longitude, radius float64) ([]models.ParkingSpot, error) {
	var spots []models.ParkingSpot

	if radius <= 0 {
		radius = 3.0
	}
	if radius > 50 {
		radius = 50.0
	}

	distanceSQL := `
        6371 * acos(
            cos(radians(?)) * cos(radians(parking_lot.latitude)) * 
            cos(radians(parking_lot.longitude) - radians(?)) + 
            sin(radians(?)) * sin(radians(parking_lot.latitude))
        )
    `

	now := time.Now().UTC()
	parsedDate, err := time.Parse("2006-01-02", date)
	if err != nil {
		return nil, fmt.Errorf("invalid date format: %w", err)
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	if parsedDate.Before(today) {
		return nil, fmt.Errorf("date must be today or in the future: %s", date)
	}

	startOfDay := time.Date(parsedDate.Year(), parsedDate.Month(), parsedDate.Day(), 0, 0, 0, 0, time.UTC)
	endOfDay := startOfDay.Add(24 * time.Hour).Add(-time.Nanosecond)

	var rentedSpotIDs []int
	if err := database.DB.Model(&models.Rent{}).
		Select("spot_id").
		Where("(actual_end_time IS NULL AND end_time >= ?) OR (end_time > ? AND start_time < ?)", now, startOfDay, endOfDay).
		Where("status NOT IN (?, ?)", "canceled", "completed").
		Distinct().
		Scan(&rentedSpotIDs).Error; err != nil {
		log.Printf("Failed to query rented spot IDs: %v", err)
		return nil, fmt.Errorf("failed to query rented spot IDs: %w", err)
	}
	log.Printf("Rented spot IDs for date %s: %v", date, rentedSpotIDs)

	query := database.DB.
		Joins("JOIN parking_lot ON parking_spot.parking_lot_id = parking_lot.parking_lot_id").
		Where("parking_spot.status = ?", "available").
		Where(distanceSQL+" <= ?", latitude, longitude, latitude, radius)

	if len(rentedSpotIDs) > 0 {
		log.Printf("Excluding rented spot IDs: %v", rentedSpotIDs)
		query = query.Where("parking_spot.spot_id NOT IN (?)", rentedSpotIDs)
	}

	err = query.Find(&spots).Error
	if err != nil {
		log.Printf("Failed to query available parking lots: %v", err)
		return nil, fmt.Errorf("failed to query available parking lots: %w", err)
	}

	if len(spots) == 0 {
		log.Printf("No parking lots found after filtering for date %s", date)
		return spots, nil
	}

	log.Printf("Successfully retrieved %d available parking lots within %f km", len(spots), radius)
	return spots, nil
}

// GetParkingLotByID 查詢特定停車場（實際查詢 parking_spot 並聯繫 parking_lot）
func GetParkingLotByID(id int) (*models.ParkingSpot, error) {
	var spot models.ParkingSpot
	if err := database.DB.
		Joins("JOIN parking_lot ON parking_spot.parking_lot_id = parking_lot.parking_lot_id").
		First(&spot, "parking_spot.spot_id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking lot with ID %d not found", id)
			return nil, nil
		}
		log.Printf("Failed to get parking lot by ID %d: %v", id, err)
		return nil, fmt.Errorf("failed to get parking lot by ID %d: %w", id, err)
	}

	log.Printf("Successfully retrieved parking lot with ID %d", id)
	return &spot, nil
}
