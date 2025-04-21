package handlers

import (
	"log"

	"github.com/gin-gonic/gin"
)

// APIResponse 定義統一的 API 回應結構
type APIResponse struct {
	Status  bool        `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`  // omitempty 表示如果為空則不顯示
	Error   string      `json:"error,omitempty"` // omitempty 表示如果為空則不顯示
	Code    string      `json:"code,omitempty"`  // 新增 Code 字段，用於錯誤碼
}

// SuccessResponse 返回成功的回應
func SuccessResponse(c *gin.Context, statusCode int, message string, data interface{}) {
	// 檢查 statusCode 是否為 2xx
	if statusCode < 200 || statusCode >= 300 {
		log.Printf("Warning: SuccessResponse called with invalid status code %d, should be 2xx", statusCode)
	}

	c.JSON(statusCode, APIResponse{
		Status:  true,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse 返回失敗的回應
func ErrorResponse(c *gin.Context, statusCode int, message string, err string, code ...string) {
	// 檢查 statusCode 是否為 4xx 或 5xx
	if statusCode < 400 || statusCode >= 600 {
		log.Printf("Warning: ErrorResponse called with invalid status code %d, should be 4xx or 5xx", statusCode)
	}

	response := APIResponse{
		Status:  false,
		Message: message,
		Error:   err,
	}

	// 如果提供了 code 參數，則設置 Code 字段
	if len(code) > 0 {
		response.Code = code[0]
	}

	c.JSON(statusCode, response)
}
