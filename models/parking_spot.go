package models

// ParkingSpot 定義停車位模型
type ParkingSpot struct {
	SpotID       int        `json:"spot_id" gorm:"primaryKey;autoIncrement;type:INT"`
	ParkingLotID int        `json:"parking_lot_id" gorm:"index;type:INT" binding:"omitempty"`
	ParkingLot   ParkingLot `json:"parking_lot" gorm:"foreignKey:ParkingLotID"`
	Status       string     `json:"status" gorm:"type:enum('available', 'occupied');default:'available'" binding:"omitempty,oneof=available occupied"`
}

func (ParkingSpot) TableName() string {
	return "parking_spot"
}

// ParkingSpotResponse 定義停車位回應結構
type ParkingSpotResponse struct {
	SpotID       int    `json:"spot_id"`
	ParkingLotID int    `json:"parking_lot_id"`
	Status       string `json:"status"`
}

func (p *ParkingSpot) ToResponse() ParkingSpotResponse {
	return ParkingSpotResponse{
		SpotID:       p.SpotID,
		ParkingLotID: p.ParkingLotID,
		Status:       p.Status,
	}
}
