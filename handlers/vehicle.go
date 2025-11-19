package handlers

import (
	"net/http"
	"project01/database"
	"project01/models"
	"project01/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetMyVehicles 取得我的所有車輛
func GetMyVehicles(c *gin.Context) {
	memberID := c.GetInt("member_id")

	vehicles, err := services.GetVehiclesByMemberID(memberID)
	if err != nil {
		ErrorResponse(c, http.StatusInternalServerError, "查詢車輛失敗", err.Error())
		return
	}

	resp := make([]models.VehicleResponse, len(vehicles))
	for i, v := range vehicles {
		resp[i] = v.ToResponse()
	}

	SuccessResponse(c, http.StatusOK, "查詢成功", resp)
}

// CreateVehicle 新增車輛
func CreateVehicle(c *gin.Context) {
	memberID := c.GetInt("member_id")

	var input struct {
		LicensePlate string  `json:"license_plate" binding:"required"`
		Brand        *string `json:"brand,omitempty"`
		Model        *string `json:"model,omitempty"`
		Color        *string `json:"color,omitempty"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "輸入格式錯誤", err.Error())
		return
	}

	vehicle := models.Vehicle{
		LicensePlate: input.LicensePlate,
		MemberID:     memberID,
		Brand:        getString(input.Brand),
		Model:        getString(input.Model),
		Color:        getString(input.Color),
		IsDefault:    false,
	}

	if err := services.CreateVehicle(&vehicle); err != nil {
		if err.Error() == "車牌 "+input.LicensePlate+" 已被使用" {
			ErrorResponse(c, http.StatusConflict, "此車牌已被其他會員註冊", err.Error())
		} else {
			ErrorResponse(c, http.StatusBadRequest, "新增車輛失敗", err.Error())
		}
		return
	}

	SuccessResponse(c, http.StatusCreated, "車輛新增成功", vehicle.ToResponse())
}

// UpdateVehicle 修改車輛（用 JSON 傳 license_plate）
func UpdateVehicle(c *gin.Context) {
	memberID := c.GetInt("member_id")

	var input struct {
		LicensePlate string  `json:"license_plate" binding:"required"`
		Brand        *string `json:"brand,omitempty"`
		Model        *string `json:"model,omitempty"`
		Color        *string `json:"color,omitempty"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "輸入格式錯誤", err.Error())
		return
	}

	updates := make(map[string]interface{})
	if input.Brand != nil {
		updates["brand"] = *input.Brand
	}
	if input.Model != nil {
		updates["model"] = *input.Model
	}
	if input.Color != nil {
		updates["color"] = *input.Color
	}

	if len(updates) == 0 {
		ErrorResponse(c, http.StatusBadRequest, "未提供任何欄位更新", "")
		return
	}

	if err := services.UpdateVehicle(input.LicensePlate, memberID, updates); err != nil {
		if err.Error() == "車牌不存在或不屬於您" {
			ErrorResponse(c, http.StatusNotFound, "車輛不存在或無權限", err.Error())
		} else {
			ErrorResponse(c, http.StatusInternalServerError, "更新失敗", err.Error())
		}
		return
	}

	var updated models.Vehicle
	database.DB.Where("license_plate = ? AND member_id = ?", input.LicensePlate, memberID).First(&updated)
	SuccessResponse(c, http.StatusOK, "車輛更新成功", updated.ToResponse())
}

// DeleteVehicle 刪除車輛（用 JSON 傳 license_plate）
func DeleteVehicle(c *gin.Context) {
	memberID := c.GetInt("member_id")

	var input struct {
		LicensePlate string `json:"license_plate" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "請提供 license_plate", err.Error())
		return
	}

	if err := services.DeleteVehicle(input.LicensePlate, memberID); err != nil {
		if err.Error() == "車牌不存在或不屬於您" {
			ErrorResponse(c, http.StatusNotFound, "車輛不存在或無權限", err.Error())
		} else {
			ErrorResponse(c, http.StatusInternalServerError, "刪除失敗", err.Error())
		}
		return
	}

	// 自動處理預設車邏輯（非同步，避免延遲）
	go func() {
		var defaultVehicle models.Vehicle
		err := database.DB.Where("member_id = ? AND is_default = ?", memberID, true).First(&defaultVehicle).Error
		if err == gorm.ErrRecordNotFound {
			// 找最早那台設為預設
			err = database.DB.Where("member_id = ?", memberID).Order("created_at asc").First(&defaultVehicle).Error
			if err == nil {
				database.DB.Model(&defaultVehicle).Update("is_default", true)
			}
		}
	}()

	SuccessResponse(c, http.StatusOK, "車輛刪除成功", nil)
}

// SetDefaultVehicle 設為預設車輛（用 JSON 傳 license_plate）
func SetDefaultVehicle(c *gin.Context) {
	memberID := c.GetInt("member_id")

	var input struct {
		LicensePlate string `json:"license_plate" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "請提供 license_plate", err.Error())
		return
	}

	if err := services.SetDefaultVehicle(input.LicensePlate, memberID); err != nil {
		ErrorResponse(c, http.StatusBadRequest, "設定失敗", err.Error())
		return
	}

	var vehicle models.Vehicle
	database.DB.Where("license_plate = ? AND member_id = ?", input.LicensePlate, memberID).First(&vehicle)

	SuccessResponse(c, http.StatusOK, "已設為預設車輛", vehicle.ToResponse())
}

// 工具函數：*string → string
func getString(ptr *string) string {
	if ptr == nil {
		return ""
	}
	return *ptr
}
