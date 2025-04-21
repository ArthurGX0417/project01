package models

import (
	"time"
)

type Rent struct {
	RentID        int         `json:"rent_id" gorm:"primaryKey;autoIncrement;type:INT"`
	MemberID      int         `json:"member_id" gorm:"index;not null;type:INT;column:member_id" binding:"required"`
	SpotID        int         `json:"spot_id" gorm:"index;not null;type:INT;column:spot_id" binding:"required"`
	StartTime     time.Time   `json:"start_time" gorm:"type:datetime;not null" binding:"required"`
	EndTime       time.Time   `json:"end_time" gorm:"type:datetime;not null" binding:"required"`
	ActualEndTime *time.Time  `json:"actual_end_time" gorm:"type:datetime"`
	TotalCost     float64     `json:"total_cost" gorm:"type:decimal(10,2);default:0.00"`
	Member        Member      `json:"-" gorm:"foreignKey:MemberID;references:MemberID"`
	ParkingSpot   ParkingSpot `json:"-" gorm:"foreignKey:SpotID;references:SpotID"`
}

func (Rent) TableName() string {
	return "rent"
}

type RentResponse struct {
	RentID        int                 `json:"rent_id"`
	MemberID      int                 `json:"member_id"`
	SpotID        int                 `json:"spot_id"`
	StartTime     time.Time           `json:"start_time"`
	EndTime       time.Time           `json:"end_time"`
	ActualEndTime *time.Time          `json:"actual_end_time"`
	TotalCost     float64             `json:"total_cost"`
	Member        MemberResponse      `json:"member"`
	ParkingSpot   ParkingSpotResponse `json:"parking_spot"`
}

type SimpleRentResponse struct {
	RentID        int        `json:"rent_id"`
	MemberID      int        `json:"member_id"`
	SpotID        int        `json:"spot_id"`
	StartTime     time.Time  `json:"start_time"`
	EndTime       time.Time  `json:"end_time"`
	ActualEndTime *time.Time `json:"actual_end_time"`
	TotalCost     float64    `json:"total_cost"`
}

func (r *Rent) ToResponse(availableDays []ParkingSpotAvailableDay, parkingSpotRents []Rent) RentResponse {
	return RentResponse{
		RentID:        r.RentID,
		MemberID:      r.MemberID,
		SpotID:        r.SpotID,
		StartTime:     r.StartTime,
		EndTime:       r.EndTime,
		ActualEndTime: r.ActualEndTime,
		TotalCost:     r.TotalCost,
		Member:        r.Member.ToResponse(),
		ParkingSpot:   r.ParkingSpot.ToResponse(availableDays, parkingSpotRents),
	}
}

func (r *Rent) ToSimpleResponse() SimpleRentResponse {
	return SimpleRentResponse{
		RentID:        r.RentID,
		MemberID:      r.MemberID,
		SpotID:        r.SpotID,
		StartTime:     r.StartTime,
		EndTime:       r.EndTime,
		ActualEndTime: r.ActualEndTime,
		TotalCost:     r.TotalCost,
	}
}
