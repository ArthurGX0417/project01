package models

import "time"

// Rent 定義租用模型
type Rent struct {
	RentID       int         `json:"rent_id" gorm:"primaryKey;autoIncrement;type:INT"`
	MemberID     int         `json:"member_id" gorm:"index;type:INT" binding:"omitempty"`
	SpotID       int         `json:"spot_id" gorm:"index;type:INT" binding:"omitempty"`
	LicensePlate string      `json:"license_plate" gorm:"type:varchar(20)" binding:"omitempty,max=20"`
	StartTime    time.Time   `json:"start_time" gorm:"type:datetime;not null" binding:"required"`
	EndTime      *time.Time  `json:"end_time" gorm:"type:datetime" binding:"omitempty"`
	TotalCost    float64     `json:"total_cost" gorm:"type:decimal(5,2)" binding:"omitempty,gte=0"`
	Status       string      `json:"status" gorm:"type:enum('pending', 'completed');default:'pending'" binding:"omitempty,oneof=pending completed"`
	Member       Member      `json:"member" gorm:"foreignKey:MemberID"`
	ParkingSpot  ParkingSpot `json:"parking_spot" gorm:"foreignKey:SpotID"`
}

func (Rent) TableName() string {
	return "rent"
}

// RentResponse 定義租用回應結構
type RentResponse struct {
	RentID       int       `json:"rent_id"`
	MemberID     int       `json:"member_id"`
	SpotID       int       `json:"spot_id"`
	LicensePlate string    `json:"license_plate"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"` // 使用 time.Time，處理 nil 情況
	TotalCost    float64   `json:"total_cost"`
	Status       string    `json:"status"`
}

type SimpleRentResponse struct {
	RentID       int       `json:"rent_id"`
	MemberID     int       `json:"member_id"`
	SpotID       int       `json:"spot_id"`
	LicensePlate string    `json:"license_plate"`
	StartTime    time.Time `json:"start_time"`
	EndTime      time.Time `json:"end_time"` // 使用 time.Time，處理 nil 情況
	TotalCost    float64   `json:"total_cost"`
	Status       string    `json:"status"`
}

func (r *Rent) ToResponse() RentResponse {
	endTime := time.Time{} // 預設零時間
	if r.EndTime != nil {
		endTime = *r.EndTime
	}
	return RentResponse{
		RentID:       r.RentID,
		MemberID:     r.MemberID,
		SpotID:       r.SpotID,
		LicensePlate: r.LicensePlate,
		StartTime:    r.StartTime,
		EndTime:      endTime,
		TotalCost:    r.TotalCost,
		Status:       r.Status,
	}
}

func (r *Rent) ToSimpleResponse() SimpleRentResponse {
	endTime := time.Time{} // 預設零時間
	if r.EndTime != nil {
		endTime = *r.EndTime
	}
	return SimpleRentResponse{
		RentID:       r.RentID,
		MemberID:     r.MemberID,
		SpotID:       r.SpotID,
		LicensePlate: r.LicensePlate,
		StartTime:    r.StartTime,
		EndTime:      endTime,
		TotalCost:    r.TotalCost,
		Status:       r.Status,
	}
}
