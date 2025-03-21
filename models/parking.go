// models/parking.go
package models

import "encoding/json"

type ParkingSpot struct {
	SpotID           int     `json:"spot_id" gorm:"primaryKey;autoIncrement;type:INT"`             // 車位ID
	MemberID         int     `json:"member_id" gorm:"index;not null;type:INT"`                     // 會員ID
	ParkingType      string  `json:"parking_type" gorm:"type:enum('mechanical', 'flat');not null"` // 機械/平面停車場
	FloorLevel       string  `json:"floor_level" gorm:"type:varchar(20)"`                          // 樓層
	Location         string  `json:"location" gorm:"type:varchar(50);not null"`                    // 位置
	PricingType      string  `json:"pricing_type" gorm:"type:enum('monthly', 'hourly');not null"`  // 月租/小時
	Status           string  `json:"status" gorm:"type:enum('in_use', 'idle');not null"`           // 使用中/空閒
	PricePerHalfHour float64 `json:"price_per_half_hour" gorm:"type:decimal(10,2);default:20.00"`  // 每半小時價格
	DailyMaxPrice    float64 `json:"daily_max_price" gorm:"type:decimal(10,2);default:300.00"`     // 每日最高費用
	Longitude        float64 `json:"longitude" gorm:"type:decimal(9,6);default:0.0"`               // 經度
	Latitude         float64 `json:"latitude" gorm:"type:decimal(9,6);default:0.0"`                // 緯度
	AvailableDays    string  `json:"available_days" gorm:"type:varchar(100)"`                      // 可用星期，存為 JSON 字串
	Member           Member  `gorm:"foreignKey:member_id;references:MemberID"`                     // 修正為小寫 member_id
	Rents            []Rent  `gorm:"foreignKey:spot_id;references:SpotID"`                         // 修正為小寫 spot_id
}

type ParkingSpotResponse struct {
	SpotID           int                  `json:"spot_id"`
	MemberID         int                  `json:"member_id"`
	ParkingType      string               `json:"parking_type"`
	FloorLevel       string               `json:"floor_level"`
	Location         string               `json:"location"`
	PricingType      string               `json:"pricing_type"`
	Status           string               `json:"status"`
	PricePerHalfHour float64              `json:"price_per_half_hour"`
	DailyMaxPrice    float64              `json:"daily_max_price"`
	Longitude        float64              `json:"longitude"`
	Latitude         float64              `json:"latitude"`
	AvailableDays    []string             `json:"available_days"` // 返回時為切片
	Member           SimpleMemberResponse `json:"Member"`
	Rents            []RentResponse       `json:"Rents"`
}

func (p *ParkingSpot) ToResponse() ParkingSpotResponse {
	rents := make([]RentResponse, len(p.Rents))
	for i, rent := range p.Rents {
		rents[i] = rent.ToResponse()
	}

	// 將 AvailableDays 從 JSON 字串解析為切片
	var availableDays []string
	if p.AvailableDays != "" {
		if err := json.Unmarshal([]byte(p.AvailableDays), &availableDays); err != nil {
			availableDays = []string{}
		}
	}

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
		AvailableDays:    availableDays,
		Member:           p.Member.ToSimpleResponse(),
		Rents:            rents,
	}
}
