package handlers

import (
	"log"
	"net/http"

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
	if statusCode < 200 || statusCode >= 300 {
		log.Printf("Warning: SuccessResponse called with invalid status code %d, should be 2xx", statusCode)
		ErrorResponse(c, http.StatusInternalServerError, "內部伺服器錯誤", "無效的成功狀態碼", "ERR_INVALID_STATUS_CODE")
		return
	}

	c.JSON(statusCode, APIResponse{
		Status:  true,
		Message: message,
		Data:    data,
	})
}

// ErrorResponse 返回失敗的回應
func ErrorResponse(c *gin.Context, statusCode int, message string, err string, code ...string) {
	if statusCode < 400 || statusCode >= 600 {
		log.Printf("Warning: ErrorResponse called with invalid status code %d, should be 4xx or 5xx", statusCode)
		statusCode = http.StatusInternalServerError // 預設為 500
	}

	response := APIResponse{
		Status:  false,
		Message: message,
		Error:   err,
	}

	if len(code) > 0 {
		response.Code = code[0]
	}

	c.JSON(statusCode, response)
}
