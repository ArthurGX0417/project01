package models

import (
	"log"
	"time"
)

type ParkingSpot struct {
	SpotID           int                       `json:"spot_id" gorm:"primaryKey;autoIncrement;type:INT"`
	MemberID         int                       `json:"member_id" gorm:"index;not null;type:INT;column:member_id" binding:"required"`
	ParkingType      string                    `json:"parking_type" gorm:"type:enum('mechanical', 'flat');not null" binding:"required,oneof=mechanical flat"`
	FloorLevel       string                    `json:"floor_level" gorm:"type:varchar(20)" binding:"omitempty,max=20"` // 可讀但不可更新
	Location         string                    `json:"location" gorm:"type:varchar(50);not null" binding:"required,max=50"`
	PricingType      string                    `json:"pricing_type" gorm:"type:enum('hourly');not null" binding:"required,oneof=hourly"` // 僅允許 "hourly"
	Status           string                    `json:"status" gorm:"type:enum('available', 'occupied', 'reserved');not null" binding:"required,oneof=available occupied reserved"`
	PricePerHalfHour float64                   `json:"price_per_half_hour" gorm:"type:decimal(10,2);default:20.00" binding:"gte=0"`
	DailyMaxPrice    float64                   `json:"daily_max_price" gorm:"type:decimal(10,2);default:300.00" binding:"gte=0"`
	Longitude        float64                   `json:"longitude" gorm:"type:decimal(9,6);default:0.0" binding:"gte=-180,lte=180"`
	Latitude         float64                   `json:"latitude" gorm:"type:decimal(9,6);default:0.0" binding:"gte=-90,lte=90"`
	Member           Member                    `json:"-" gorm:"foreignKey:MemberID;references:MemberID"`
	Rents            []Rent                    `gorm:"foreignKey:SpotID;references:SpotID"`
	AvailableDays    []ParkingSpotAvailableDay `json:"-" gorm:"foreignKey:SpotID;references:SpotID"`
}

func (ParkingSpot) TableName() string {
	return "parking_spot"
}

type ParkingSpotResponse struct {
	SpotID           int                    `json:"spot_id"`
	MemberID         int                    `json:"member_id"`
	ParkingType      string                 `json:"parking_type"`
	FloorLevel       string                 `json:"floor_level"`
	Location         string                 `json:"location"`
	PricingType      string                 `json:"pricing_type"`
	Status           string                 `json:"status"`
	PricePerHalfHour float64                `json:"price_per_half_hour"`
	DailyMaxPrice    float64                `json:"daily_max_price"`
	Longitude        float64                `json:"longitude"`
	Latitude         float64                `json:"latitude"`
	AvailableDays    []AvailableDayResponse `json:"available_days"`
	Member           MemberResponse         `json:"member"`
	Rents            []SimpleRentResponse   `json:"rents"`
}

func (p *ParkingSpot) ToResponse(availableDays []ParkingSpotAvailableDay, rents []Rent) ParkingSpotResponse {
	now := time.Now().UTC()
	var activeRents []Rent
	for _, rent := range rents {
		if rent.ActualEndTime == nil && rent.EndTime.After(now) {
			activeRents = append(activeRents, rent)
		}
	}

	rentResponses := make([]SimpleRentResponse, len(activeRents))
	for i, rent := range activeRents {
		rentResponses[i] = rent.ToSimpleResponse()
	}

	days := make([]AvailableDayResponse, len(availableDays))
	for i, day := range availableDays {
		days[i] = day.ToResponse()
	}

	// 添加日誌檢查價格值
	log.Printf("Converting ParkingSpot %d to response: PricePerHalfHour=%.2f, DailyMaxPrice=%.2f",
		p.SpotID, p.PricePerHalfHour, p.DailyMaxPrice)

	return ParkingSpotResponse{
		SpotID:           p.SpotID,
		MemberID:         p.MemberID,
		ParkingType:      p.ParkingType,
		FloorLevel:       p.FloorLevel,
		Location:         p.Location,
		PricingType:      p.PricingType,
		Status:           p.Status,
		PricePerHalfHour: p.PricePerHalfHour,
		DailyMaxPrice:    p.DailyMaxPrice,
		Longitude:        p.Longitude,
		Latitude:         p.Latitude,
		AvailableDays:    days,
		Member:           p.Member.ToResponse(),
		Rents:            rentResponses,
	}
}
