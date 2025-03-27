package models

func (Member) TableName() string {
	return "member"
}

type Member struct {
	MemberID           int           `json:"member_id" gorm:"primaryKey;autoIncrement;type:INT"`
	Name               string        `json:"name" gorm:"type:varchar(50);not null" binding:"required,max=50"`
	Phone              string        `json:"phone" gorm:"type:varchar(20);not null" binding:"required,max=20"`
	Password           string        `json:"password" gorm:"type:varchar(100);not null" binding:"required,min=8,max=100"`
	Role               string        `json:"role" gorm:"type:enum('shared_owner', 'renter');not null" binding:"required,oneof=shared_owner renter"`
	PaymentMethod      string        `json:"payment_method" gorm:"type:enum('credit_card', 'e_wallet');not null" binding:"required,oneof=credit_card e_wallet"`
	PaymentInfo        string        `json:"payment_info" gorm:"type:varchar(100)" binding:"omitempty,max=100"`
	AutoMonthlyPayment bool          `json:"auto_monthly_payment" gorm:"type:tinyint(1);default:0"`
	LicensePlate       string        `json:"license_plate" gorm:"type:varchar(20)" binding:"omitempty,max=20"`
	CarModel           string        `json:"car_model" gorm:"type:varchar(50)" binding:"omitempty,max=50"`
	Email              string        `json:"email" gorm:"type:varchar(100);not null" binding:"required,email,max=100"`
	WifiVerified       bool          `json:"wifi_verified" gorm:"type:tinyint(1);default:0"`
	Spots              []ParkingSpot `json:"-" gorm:"foreignKey:member_id;references:MemberID"`
	Rents              []Rent        `json:"-" gorm:"foreignKey:member_id;references:MemberID"`
}

type SimpleMemberResponse struct {
	MemberID           int    `json:"member_id"`
	Name               string `json:"name"`
	Phone              string `json:"phone"`
	Role               string `json:"role"`
	PaymentMethod      string `json:"payment_method"`
	AutoMonthlyPayment bool   `json:"auto_monthly_payment"`
	LicensePlate       string `json:"license_plate"`
	CarModel           string `json:"car_model"`
	Email              string `json:"email"`
	WifiVerified       bool   `json:"wifi_verified"`
}

type MemberResponse struct {
	MemberID           int                   `json:"member_id"`
	Name               string                `json:"name"`
	Phone              string                `json:"phone"`
	Role               string                `json:"role"`
	PaymentMethod      string                `json:"payment_method"`
	PaymentInfo        string                `json:"payment_info"`
	AutoMonthlyPayment bool                  `json:"auto_monthly_payment"`
	LicensePlate       string                `json:"license_plate"`
	CarModel           string                `json:"car_model"`
	Email              string                `json:"email"`
	WifiVerified       bool                  `json:"wifi_verified"`
	Spots              []ParkingSpotResponse `json:"spots,omitempty"`
}

func (m *Member) ToResponse() MemberResponse {
	return MemberResponse{
		MemberID:           m.MemberID,
		Name:               m.Name,
		Phone:              m.Phone,
		Role:               m.Role,
		PaymentMethod:      m.PaymentMethod,
		PaymentInfo:        m.PaymentInfo,
		AutoMonthlyPayment: m.AutoMonthlyPayment,
		LicensePlate:       m.LicensePlate,
		CarModel:           m.CarModel,
		Email:              m.Email,
		WifiVerified:       m.WifiVerified,
	}
}

func (m *Member) ToResponseWithSpots(spots []ParkingSpot) MemberResponse {
	spotsResponse := make([]ParkingSpotResponse, len(spots))
	for i, spot := range spots {
		var availableDays []ParkingSpotAvailableDay
		for _, record := range spot.AvailableDays {
			availableDays = append(availableDays, record)
		}
		spotsResponse[i] = spot.ToResponse(availableDays)
	}

	return MemberResponse{
		MemberID:           m.MemberID,
		Name:               m.Name,
		Phone:              m.Phone,
		Role:               m.Role,
		PaymentMethod:      m.PaymentMethod,
		PaymentInfo:        m.PaymentInfo,
		AutoMonthlyPayment: m.AutoMonthlyPayment,
		LicensePlate:       m.LicensePlate,
		CarModel:           m.CarModel,
		Email:              m.Email,
		WifiVerified:       m.WifiVerified,
		Spots:              spotsResponse,
	}
}
