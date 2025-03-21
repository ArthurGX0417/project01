package models

import "project01/database"

type Member struct {
	MemberID           int           `json:"member_id" gorm:"primaryKey;autoIncrement;type:INT"`
	Name               string        `json:"name" gorm:"type:varchar(50);not null" binding:"required"`
	Phone              string        `json:"phone" gorm:"type:varchar(20);not null;unique" binding:"required,len=10"`
	Password           string        `json:"-" gorm:"type:varchar(100);not null" binding:"required,min=8"`
	Role               string        `json:"role" gorm:"type:enum('renter', 'shared_owner');not null" binding:"required,oneof=renter shared_owner"`
	PaymentMethod      string        `json:"payment_method" gorm:"type:enum('credit_card', 'e_wallet');not null" binding:"required,oneof=credit_card e_wallet"`
	PaymentInfo        string        `json:"-" gorm:"type:varchar(100);not null" binding:"required"`
	AutoMonthlyPayment bool          `json:"auto_monthly_payment" gorm:"default:false" binding:"boolean"`
	LicensePlate       string        `json:"license_plate" gorm:"type:varchar(20)" binding:"omitempty,max=20"`
	CarModel           string        `json:"car_model" gorm:"type:varchar(50)" binding:"omitempty,max=50"`
	Email              string        `json:"email" gorm:"type:varchar(100);not null;unique" binding:"required,email"`
	WifiVerified       bool          `json:"wifi_verified" gorm:"default:false" binding:"boolean"`
	Rents              []Rent        `json:"rents" gorm:"foreignKey:member_id;references:MemberID"`
	ParkingSpots       []ParkingSpot `json:"parking_spots" gorm:"foreignKey:member_id;references:MemberID"`
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
	AutoMonthlyPayment bool                  `json:"auto_monthly_payment"`
	LicensePlate       string                `json:"license_plate"`
	CarModel           string                `json:"car_model"`
	Email              string                `json:"email"`
	WifiVerified       bool                  `json:"wifi_verified"`
	Rents              []RentResponse        `json:"rents"`
	ParkingSpots       []ParkingSpotResponse `json:"parking_spots"`
}

func (m *Member) ToResponse() MemberResponse {
	// Fetch all available days for parking spots in rents
	rentSpotIDs := make([]int, len(m.Rents))
	for i, rent := range m.Rents {
		rentSpotIDs[i] = rent.SpotID
	}

	var rentAvailableDaysRecords []ParkingSpotAvailableDay
	rentAvailableDaysMap := make(map[int][]string)
	if len(rentSpotIDs) > 0 {
		if err := database.DB.Where("spot_id IN ?", rentSpotIDs).Find(&rentAvailableDaysRecords).Error; err != nil {
			// Log the error but continue with empty days
			rentAvailableDaysRecords = []ParkingSpotAvailableDay{}
		}
		for _, record := range rentAvailableDaysRecords {
			rentAvailableDaysMap[record.SpotID] = append(rentAvailableDaysMap[record.SpotID], record.Day)
		}
	}

	rents := make([]RentResponse, len(m.Rents))
	for i, rent := range m.Rents {
		availableDays := rentAvailableDaysMap[rent.SpotID]
		if availableDays == nil {
			availableDays = []string{}
		}
		rents[i] = rent.ToResponse(availableDays)
	}

	// Fetch all available days for all parking spots in a single query
	spotIDs := make([]int, len(m.ParkingSpots))
	for i, spot := range m.ParkingSpots {
		spotIDs[i] = spot.SpotID
	}

	var availableDaysRecords []ParkingSpotAvailableDay
	availableDaysMap := make(map[int][]string)
	if len(spotIDs) > 0 {
		if err := database.DB.Where("spot_id IN ?", spotIDs).Find(&availableDaysRecords).Error; err != nil {
			// Log the error but continue with empty days
			availableDaysRecords = []ParkingSpotAvailableDay{}
		}
		for _, record := range availableDaysRecords {
			availableDaysMap[record.SpotID] = append(availableDaysMap[record.SpotID], record.Day)
		}
	}

	parkingSpots := make([]ParkingSpotResponse, len(m.ParkingSpots))
	for i, spot := range m.ParkingSpots {
		availableDays := availableDaysMap[spot.SpotID]
		if availableDays == nil {
			availableDays = []string{}
		}
		parkingSpots[i] = spot.ToResponse(availableDays)
	}

	return MemberResponse{
		MemberID:           m.MemberID,
		Name:               m.Name,
		Phone:              m.Phone,
		Role:               m.Role,
		PaymentMethod:      m.PaymentMethod,
		AutoMonthlyPayment: m.AutoMonthlyPayment,
		LicensePlate:       m.LicensePlate,
		CarModel:           m.CarModel,
		Email:              m.Email,
		WifiVerified:       m.WifiVerified,
		Rents:              rents,
		ParkingSpots:       parkingSpots,
	}
}

func (m *Member) ToSimpleResponse() SimpleMemberResponse {
	return SimpleMemberResponse{
		MemberID:           m.MemberID,
		Name:               m.Name,
		Phone:              m.Phone,
		Role:               m.Role,
		PaymentMethod:      m.PaymentMethod,
		AutoMonthlyPayment: m.AutoMonthlyPayment,
		LicensePlate:       m.LicensePlate,
		CarModel:           m.CarModel,
		Email:              m.Email,
		WifiVerified:       m.WifiVerified,
	}
}
