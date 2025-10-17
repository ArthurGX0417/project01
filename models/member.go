package models

// Member 定義會員模型
type Member struct {
	MemberID     int    `json:"member_id" gorm:"primaryKey;autoIncrement;type:INT"`
	Email        string `json:"email" gorm:"type:varchar(50);unique" binding:"omitempty,email,max=50"`
	Phone        string `json:"phone" gorm:"type:varchar(10);unique" binding:"omitempty,max=10"`
	Password     string `json:"password" gorm:"type:varchar(100)" binding:"omitempty,min=8,max=100"`
	LicensePlate string `json:"license_plate" gorm:"type:varchar(20);unique" binding:"omitempty,max=20"`
	PaymentInfo  string `json:"payment_info" gorm:"type:varchar(100)" binding:"omitempty,max=100"`
	Role         string `json:"role" gorm:"type:enum('renter', 'admin');default:'renter'" binding:"omitempty,oneof=renter admin"`
}

func (Member) TableName() string {
	return "member"
}

// MemberResponse 定義會員回應結構
type MemberResponse struct {
	MemberID     int    `json:"member_id"`
	Email        string `json:"email"`
	Phone        string `json:"phone"`
	LicensePlate string `json:"license_plate"`
	PaymentInfo  string `json:"payment_info"`
	Role         string `json:"role"`
}

func (m *Member) ToResponse() MemberResponse {
	return MemberResponse{
		MemberID:     m.MemberID,
		Email:        m.Email,
		Phone:        m.Phone,
		LicensePlate: m.LicensePlate,
		PaymentInfo:  m.PaymentInfo,
		Role:         m.Role,
	}
}
