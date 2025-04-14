package models

type ParkingSpot struct {
	SpotID           int                       `json:"spot_id" gorm:"primaryKey;autoIncrement;type:INT"`
	MemberID         int                       `json:"member_id" gorm:"index;not null;type:INT;column:member_id" binding:"required"`
	ParkingType      string                    `json:"parking_type" gorm:"type:enum('mechanical', 'flat');not null" binding:"required,oneof=mechanical flat"`
	FloorLevel       string                    `json:"floor_level" gorm:"type:varchar(20)" binding:"omitempty,max=20"`
	Location         string                    `json:"location" gorm:"type:varchar(50);not null" binding:"required,max=50"`
	PricingType      string                    `json:"pricing_type" gorm:"type:enum('monthly', 'hourly');not null" binding:"required,oneof=monthly hourly"`
	Status           string                    `json:"status" gorm:"type:enum('in_use', 'idle');not null" binding:"required,oneof=in_use idle"`
	PricePerHalfHour float64                   `json:"price_per_half_hour" gorm:"type:decimal(10,2);default:20.00" binding:"gte=0"`
	DailyMaxPrice    float64                   `json:"daily_max_price" gorm:"type:decimal(10,2);default:300.00" binding:"gte=0"`
	Longitude        float64                   `json:"longitude" gorm:"type:decimal(9,6);default:0.0" binding:"gte=-180,lte=180"`
	Latitude         float64                   `json:"latitude" gorm:"type:decimal(9,6);default:0.0" binding:"gte=-90,lte=90"`
	Member           Member                    `json:"-" gorm:"foreignKey:MemberID;references:MemberID"`
	Rents            []Rent                    `json:"-" gorm:"foreignKey:spot_id;references:SpotID"`
	AvailableDays    []ParkingSpotAvailableDay `json:"-" gorm:"foreignKey:SpotID;references:SpotID"`
}

func (ParkingSpot) TableName() string {
	return "parking_spot"
}

type AvailableDayResponse struct {
	Date        string `json:"date"`
	IsAvailable bool   `json:"is_available"`
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
	Rents            []RentResponse         `json:"rents"`
}

func (p *ParkingSpot) ToResponse(availableDays []ParkingSpotAvailableDay) ParkingSpotResponse {
	rents := make([]RentResponse, len(p.Rents))
	for i, rent := range p.Rents {
		rents[i] = rent.ToResponse(nil) // Pass nil since RentResponse doesn't need availableDays
	}

	days := make([]AvailableDayResponse, len(availableDays))
	for i, day := range availableDays {
		days[i] = day.ToResponse() // 使用 ParkingSpotAvailableDay 的 ToResponse 方法
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
		AvailableDays:    days,
		Member:           p.Member.ToResponse(),
		Rents:            rents,
	}
}
