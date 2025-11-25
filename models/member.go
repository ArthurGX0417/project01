package models

type Member struct {
	MemberID    int    `json:"member_id" gorm:"primaryKey;autoIncrement;type:INT"`
	Email       string `json:"email" gorm:"type:varchar(50);unique" binding:"omitempty,email,max=50"`
	Phone       string `json:"phone" gorm:"type:varchar(10);unique" binding:"omitempty,max=10"`
	Password    string `json:"-" gorm:"type:varchar(100)"`
	PaymentInfo string `json:"payment_info" gorm:"type:varchar(100)"`
	Role        string `json:"role" gorm:"type:enum('renter', 'admin');default:'renter'"`
	Name        string `json:"name" gorm:"type:varchar(50)"`
}

func (Member) TableName() string {
	return "member"
}

type MemberResponse struct {
	MemberID    int    `json:"member_id"`
	Email       string `json:"email"`
	Phone       string `json:"phone"`
	PaymentInfo string `json:"payment_info"`
	Role        string `json:"role"`
	Name        string `json:"name"`
}

func (m *Member) ToResponse() MemberResponse {
	return MemberResponse{
		MemberID:    m.MemberID,
		Email:       m.Email,
		Phone:       m.Phone,
		PaymentInfo: m.PaymentInfo,
		Role:        m.Role,
		Name:        m.Name,
	}
}
