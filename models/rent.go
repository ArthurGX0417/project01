// models/rent.go
package models

import (
	"fmt"
	"time"
)

type Rent struct {
	LicensePlate string     `gorm:"primaryKey;size:20;column:license_plate" json:"license_plate"`
	StartTime    time.Time  `gorm:"primaryKey;column:start_time" json:"start_time"`
	SpotID       *int       `gorm:"column:spot_id" json:"spot_id,omitempty"`
	EndTime      *time.Time `gorm:"column:end_time" json:"end_time,omitempty"`
	TotalCost    *float64   `gorm:"type:decimal(5,2);column:total_cost" json:"total_cost,omitempty"`
	Status       string     `gorm:"type:enum('pending','completed');default:'pending';column:status" json:"status"`

	// 保留車位關聯（您目前還需要知道停哪格）
	Spot ParkingSpot `gorm:"foreignKey:SpotID;references:SpotID" json:"parking_spot,omitempty"`
}

func (Rent) TableName() string {
	return "rent"
}

// 回應結構（前端要的格式）
type RentResponse struct {
	LicensePlate string  `json:"license_plate"`
	StartTime    string  `json:"start_time"`
	SpotID       *int    `json:"spot_id,omitempty"`
	EndTime      *string `json:"end_time,omitempty"`
	TotalCost    *string `json:"total_cost,omitempty"`
	Status       string  `json:"status"`
}

// 轉換方法
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
		StartTime:    r.StartTime.Format(time.RFC3339),
		SpotID:       r.SpotID,
		EndTime:      endTimeStr,
		TotalCost:    costStr,
		Status:       r.Status,
	}
}
