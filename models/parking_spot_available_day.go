package models

import "time"

type ParkingSpotAvailableDay struct {
	ID            int       `json:"id" gorm:"primaryKey;autoIncrement;type:INT"`
	SpotID        int       `json:"parking_spot_id" gorm:"index;not null;type:INT;column:parking_spot_id" binding:"required"`
	AvailableDate time.Time `json:"available_date" gorm:"type:date;not null" binding:"required"`
	IsAvailable   bool      `json:"is_available" gorm:"type:tinyint(1);not null;default:1" binding:"required"`
}

func (ParkingSpotAvailableDay) TableName() string {
	return "parking_spot_available_day"
}

type AvailableDayResponse struct {
	Date        string `json:"date"`
	IsAvailable bool   `json:"is_available"`
}

func (p *ParkingSpotAvailableDay) ToResponse() AvailableDayResponse {
	return AvailableDayResponse{
		Date:        p.AvailableDate.Format("2006-01-02"),
		IsAvailable: p.IsAvailable,
	}
}
