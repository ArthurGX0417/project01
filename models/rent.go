package models

import "time"

type Rent struct {
	RentID        int         `json:"rent_id" gorm:"primaryKey;autoIncrement;type:INT"`                            // 租用ID
	MemberID      int         `json:"member_id" gorm:"index;not null;type:INT" binding:"required,gt=0"`            // 會員ID
	SpotID        int         `json:"spot_id" gorm:"index;not null;type:INT" binding:"required,gt=0"`              // 車位ID
	StartTime     time.Time   `json:"start_time" gorm:"type:datetime;not null" binding:"required"`                 // 開始時間/日期
	EndTime       time.Time   `json:"end_time" gorm:"type:datetime;not null" binding:"required,gtfield=StartTime"` // 結束時間/日期
	ActualEndTime *time.Time  `json:"actual_end_time" gorm:"type:datetime;default:null" binding:"omitempty"`       // 實際離開時間
	TotalCost     float64     `json:"total_cost" gorm:"type:decimal(10,2);default:0.0" binding:"gte=0"`            // 總費用
	Member        Member      `json:"-" gorm:"foreignKey:member_id;references:MemberID"`                           // 修正為小寫 member_id
	ParkingSpot   ParkingSpot `json:"-" gorm:"foreignKey:spot_id;references:SpotID"`                               // 修正為小寫 spot_id
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

type RentResponse struct {
	RentID        int                  `json:"rent_id"`
	MemberID      int                  `json:"member_id"`
	SpotID        int                  `json:"spot_id"`
	StartTime     time.Time            `json:"start_time"`
	EndTime       time.Time            `json:"end_time"`
	ActualEndTime *time.Time           `json:"actual_end_time"`
	TotalCost     float64              `json:"total_cost"`
	Member        SimpleMemberResponse `json:"member"`       // Changed to lowercase "member"
	ParkingSpot   ParkingSpotResponse  `json:"parking_spot"` // Changed to lowercase "parking_spot"
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

func (r *Rent) ToResponse(availableDays []string) RentResponse {
	return RentResponse{
		RentID:        r.RentID,
		MemberID:      r.MemberID,
		SpotID:        r.SpotID,
		StartTime:     r.StartTime,
		EndTime:       r.EndTime,
		ActualEndTime: r.ActualEndTime,
		TotalCost:     r.TotalCost,
		Member:        r.Member.ToSimpleResponse(),
		ParkingSpot:   r.ParkingSpot.ToResponse(availableDays),
	}
}
