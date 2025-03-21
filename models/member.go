package models

type Member struct {
    MemberID           int           `json:"member_id" gorm:"primaryKey;autoIncrement;type:INT"`
    Name               string        `json:"name" gorm:"type:varchar(50);not null"`
    Phone              string        `json:"phone" gorm:"type:varchar(20);not null"`
    Password           string        `json:"password" gorm:"type:varchar(100);not null"`
    Role               string        `json:"role" gorm:"type:enum('shared_owner', 'renter');not null"`
    PaymentMethod      string        `json:"payment_method" gorm:"type:enum('credit_card', 'e_wallet');not null"`
    PaymentInfo        string        `json:"payment_info" gorm:"type:varchar(100)"`
    AutoMonthlyPayment int           `json:"auto_monthly_payment" gorm:"type:tinyint(1);default:0"`
    LicensePlate       string        `json:"license_plate" gorm:"type:varchar(20)"`
    CarModel           string        `json:"car_model" gorm:"type:varchar(50)"`
    Email              string        `json:"email" gorm:"type:varchar(100);not null"`
    WifiVerified       int           `json:"wifi_verified" gorm:"type:tinyint(1);default:0"`
    ParkingSpots       []ParkingSpot `gorm:"foreignKey:member_id;references:MemberID"`
    Rents              []Rent        `gorm:"foreignKey:member_id;references:MemberID"`
}

type SimpleMemberResponse struct {
    MemberID           int    `json:"member_id"`
    Name               string `json:"name"`
    Phone              string `json:"phone"`
    Role               string `json:"role"`
    PaymentMethod      string `json:"payment_method"`
    AutoMonthlyPayment int    `json:"auto_monthly_payment"`
    LicensePlate       string `json:"license_plate"`
    CarModel           string `json:"car_model"`
    Email              string `json:"email"`
    WifiVerified       int    `json:"wifi_verified"`
}

type MemberResponse struct {
    MemberID           int                   `json:"member_id"`
    Name               string                `json:"name"`
    Phone              string                `json:"phone"`
    Role               string                `json:"role"`
    PaymentMethod      string                `json:"payment_method"`
    AutoMonthlyPayment int                   `json:"auto_monthly_payment"`
    LicensePlate       string                `json:"license_plate"`
    CarModel           string                `json:"car_model"`
    Email              string                `json:"email"`
    WifiVerified       int                   `json:"wifi_verified"`
    ParkingSpots       []ParkingSpotResponse `json:"ParkingSpots"`
    Rents              []RentResponse        `json:"Rents"`
}

func (m *Member) ToResponse() MemberResponse {
    parkingSpots := make([]ParkingSpotResponse, len(m.ParkingSpots))
    for i, spot := range m.ParkingSpots {
        parkingSpots[i] = spot.ToResponse()
    }

    rents := make([]RentResponse, len(m.Rents))
    for i, rent := range m.Rents {
        rents[i] = rent.ToResponse()
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
        ParkingSpots:       parkingSpots,
        Rents:              rents,
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
