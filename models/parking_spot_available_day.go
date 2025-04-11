package models

import "time"

type ParkingSpotAvailableDay struct {
	ID            int       `json:"id" gorm:"primaryKey"`
	SpotID        int       `json:"spot_id" gorm:"column:parking_spot_id"`
	AvailableDate time.Time `json:"available_date" gorm:"type:date"` // 改為 time.Time
	IsAvailable   bool      `json:"is_available" gorm:"type:tinyint(1);default:1"`
}

// TableName 指定表名稱為 parking_spot_available_day
func (ParkingSpotAvailableDay) TableName() string {
	return "parking_spot_available_day"
}

// ToResponse 將 ParkingSpotAvailableDay 轉換為 AvailableDayResponse
func (day *ParkingSpotAvailableDay) ToResponse() AvailableDayResponse {
	return AvailableDayResponse{
		Date:        day.AvailableDate.Format("2006-01-02"), // 格式化為 YYYY-MM-DD
		IsAvailable: day.IsAvailable,
	}
}
