// models/rent.go
package models

import (
	"fmt"
	"time"
)

type Rent struct {
	LicensePlate string     `gorm:"primaryKey;size:20;column:license_plate" json:"license_plate"`
	ParkingLotID int        `gorm:"column:parking_lot_id" json:"parking_lot_id"`
	StartTime    time.Time  `gorm:"primaryKey;column:start_time" json:"start_time"`
	EndTime      *time.Time `gorm:"column:end_time" json:"end_time,omitempty"`
	TotalCost    *float64   `gorm:"type:decimal(6,2);column:total_cost" json:"total_cost,omitempty"`
	Status       string     `gorm:"type:enum('pending','completed');default:'pending';column:status" json:"status"`
	ParkingLot   ParkingLot `gorm:"foreignKey:ParkingLotID;references:ParkingLotID" json:"parking_lot,omitempty"`
}

func (Rent) TableName() string {
	return "rent"
}

// 前端回應（可選）
type RentResponse struct {
	LicensePlate string  `json:"license_plate"`
	ParkingLotID int     `json:"parking_lot_id"`
	StartTime    string  `json:"start_time"`
	EndTime      *string `json:"end_time,omitempty"`
	TotalCost    *string `json:"total_cost,omitempty"`
	Status       string  `json:"status"`
}

func (r *Rent) ToResponse() RentResponse {
	var endTimeStr, costStr *string
	if r.EndTime != nil {
		s := r.EndTime.Format(time.RFC3339)
		endTimeStr = &s
	}
	if r.TotalCost != nil {
		s := fmt.Sprintf("%.2f", *r.TotalCost)
		costStr = &s
	}
	return RentResponse{
		LicensePlate: r.LicensePlate,
		ParkingLotID: r.ParkingLotID,
		StartTime:    r.StartTime.Format(time.RFC3339),
		EndTime:      endTimeStr,
		TotalCost:    costStr,
		Status:       r.Status,
	}
}
