package models

type ParkingSpotAvailableDay struct {
	ID     int         `json:"id" gorm:"primaryKey;autoIncrement;type:INT"`
	SpotID int         `json:"spot_id" gorm:"not null;type:INT;uniqueIndex:idx_spot_day" binding:"required,gt=0"`
	Day    string      `json:"day" gorm:"type:enum('Monday', 'Tuesday', 'Wednesday', 'Thursday', 'Friday', 'Saturday', 'Sunday');not null;uniqueIndex:idx_spot_day" binding:"required,oneof=Monday Tuesday Wednesday Thursday Friday Saturday Sunday"`
	Spot   ParkingSpot `json:"-" gorm:"foreignKey:spot_id;references:SpotID"`
}
