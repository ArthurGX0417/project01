package models

import "time"

type Rent struct {
	RentID        int         `json:"rent_id" gorm:"primaryKey;autoIncrement;type:INT"`  // 租用ID
	MemberID      int         `json:"member_id" gorm:"index;not null;type:INT"`          // 會員ID
	SpotID        int         `json:"spot_id" gorm:"index;not null;type:INT"`            // 車位ID
	StartTime     time.Time   `json:"start_time" gorm:"not null"`                        // 開始時間/日期
	EndTime       time.Time   `json:"end_time" gorm:"not null"`                          // 結束時間/日期
	ActualEndTime *time.Time  `json:"actual_end_time" gorm:"type:datetime;default:null"` // 實際離開時間
	TotalCost     float64     `json:"total_cost" gorm:"type:decimal(10,2);default:0.0"`  // 總費用
	Member        Member      `gorm:"foreignKey:member_id;references:MemberID"`          // 修正為小寫 member_id
	ParkingSpot   ParkingSpot `gorm:"foreignKey:spot_id;references:SpotID"`              // 修正為小寫 spot_id
}

type RentResponse struct {
	RentID        int                  `json:"rent_id"`
	MemberID      int                  `json:"member_id"`
	SpotID        int                  `json:"spot_id"`
	StartTime     time.Time            `json:"start_time"`
	EndTime       time.Time            `json:"end_time"`
	ActualEndTime *time.Time           `json:"actual_end_time"`
	TotalCost     float64              `json:"total_cost"`
	Member        SimpleMemberResponse `json:"Member"`
	ParkingSpot   ParkingSpotResponse  `json:"ParkingSpot"`
}

func (r *Rent) ToResponse() RentResponse {
	return RentResponse{
		RentID:        r.RentID,
		MemberID:      r.MemberID,
		SpotID:        r.SpotID,
		StartTime:     r.StartTime,
		EndTime:       r.EndTime,
		ActualEndTime: r.ActualEndTime,
		TotalCost:     r.TotalCost,
		Member:        r.Member.ToSimpleResponse(),
		ParkingSpot:   r.ParkingSpot.ToResponse(),
	}
}
