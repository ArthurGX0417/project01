package models

type ParkingSpotAvailableDay struct {
	ID            int    `json:"id" gorm:"primaryKey"`
	SpotID        int    `json:"spot_id" gorm:"column:parking_spot_id"`
	AvailableDate string `json:"available_date" gorm:"type:date"`
	IsAvailable   bool   `json:"is_available" gorm:"type:tinyint(1);default:1"`
}
