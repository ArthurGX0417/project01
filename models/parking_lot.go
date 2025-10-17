package models

// ParkingLot 定義停車場模型
type ParkingLot struct {
	ParkingLotID int           `json:"parking_lot_id" gorm:"primaryKey;autoIncrement;type:INT"`
	Type         string        `json:"type" gorm:"type:enum('flat', 'mechanical')" binding:"omitempty,oneof=flat mechanical"`
	Address      string        `json:"address" gorm:"type:varchar(100)" binding:"omitempty,max=100"`
	HourlyRate   float64       `json:"hourly_rate" gorm:"type:decimal(10,2)" binding:"omitempty,gte=0"`
	TotalSpots   int           `json:"total_spots" gorm:"type:INT" binding:"omitempty,gte=0"`
	Longitude    float64       `json:"longitude" gorm:"type:decimal(9,6)" binding:"omitempty,gte=-180,lte=180"`
	Latitude     float64       `json:"latitude" gorm:"type:decimal(9,6)" binding:"omitempty,gte=-90,lte=90"`
	ParkingSpots []ParkingSpot `json:"parking_spots" gorm:"foreignKey:ParkingLotID"`
}

func (ParkingLot) TableName() string {
	return "parking_lot"
}

// ParkingLotResponse 定義停車場回應結構
type ParkingLotResponse struct {
	ParkingLotID int     `json:"parking_lot_id"`
	Type         string  `json:"type"`
	Address      string  `json:"address"`
	HourlyRate   float64 `json:"hourly_rate"`
	TotalSpots   int     `json:"total_spots"`
	Longitude    float64 `json:"longitude"`
	Latitude     float64 `json:"latitude"`
}

func (p *ParkingLot) ToResponse() ParkingLotResponse {
	return ParkingLotResponse{
		ParkingLotID: p.ParkingLotID,
		Type:         p.Type,
		Address:      p.Address,
		HourlyRate:   p.HourlyRate,
		TotalSpots:   p.TotalSpots,
		Longitude:    p.Longitude,
		Latitude:     p.Latitude,
	}
}
