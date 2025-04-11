package handlers

import (
	"github.com/gin-gonic/gin"
)

// APIResponse 定義統一的 API 回應結構
type APIResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"` // omitempty 表示如果為空則不顯示
	Error   string      `json:"error,omitempty"`
}

// SuccessResponse 返回成功的回應
func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
	c.JSON(statusCode, APIResponse{
		Status:  true,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse 返回失敗的回應
func ErrorResponse(c *gin.Context, statusCode int, message string, err string) {
	c.JSON(statusCode, APIResponse{
		Status:  false,
		Message: message,
		Error:   err,
	})
}
