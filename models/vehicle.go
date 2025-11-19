package models

import "time"

// Vehicle 車輛表：支援一人多車 + 預設車牌
type Vehicle struct {
	LicensePlate string `gorm:"primaryKey;size:20;column:license_plate" json:"license_plate" binding:"required"`
	MemberID     int    `gorm:"column:member_id;index:idx_member" json:"member_id" binding:"required"`
	Brand        string `gorm:"size:50;column:brand" json:"brand,omitempty"`
	Model        string `gorm:"size:50;column:model" json:"model,omitempty"`
	Color        string `gorm:"size:20;column:color" json:"color,omitempty"`
	IsDefault    bool   `gorm:"column:is_default;default:false" json:"is_default"`

	// 關聯：這台車屬於哪個會員（可選 Preload）
	Member Member `gorm:"foreignKey:MemberID;references:MemberID" json:"member,omitempty"`

	// 時間欄位（GORM 自動管理）
	CreatedAt time.Time  `gorm:"column:created_at" json:"created_at"`
	UpdatedAt time.Time  `gorm:"column:updated_at" json:"updated_at"`
	DeletedAt *time.Time `gorm:"column:deleted_at;index" json:"deleted_at,omitempty"`
}

// TableName 指定表名（可選，預設就是 vehicle）
func (Vehicle) TableName() string {
	return "vehicle"
}

// 給前端用的回應結構（可選，美觀用）
type VehicleResponse struct {
	LicensePlate string `json:"license_plate"`
	Brand        string `json:"brand,omitempty"`
	Model        string `json:"model,omitempty"`
	Color        string `json:"color,omitempty"`
	IsDefault    bool   `json:"is_default"`
	CreatedAt    string `json:"created_at"`
}

func (v *Vehicle) ToResponse() VehicleResponse {
	return VehicleResponse{
		LicensePlate: v.LicensePlate,
		Brand:        v.Brand,
		Model:        v.Model,
		Color:        v.Color,
		IsDefault:    v.IsDefault,
		CreatedAt:    v.CreatedAt.Format("2006-01-02 15:04:05"),
	}
}
