package routes

import (
	"net/http"
	"os"
	"project01/handlers"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5" // 更新為 jwt/v5
)

// 定義一個密鑰，用於簽署和驗證 JWT
var jwtSecret = []byte(os.Getenv("JWT_SECRET"))

func init() {
	if len(jwtSecret) == 0 {
		panic("JWT_SECRET environment variable is not set")
	}
}

// AuthMiddleware 驗證 JWT token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// 從 Authorization 標頭中獲取 token
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "缺少 Authorization 標頭"})
			c.Abort()
			return
		}

		// Authorization 標頭格式應為 "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "無效的 Authorization 格式"})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 解析並驗證 JWT
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			// 確保簽署方法是 HMAC
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return jwtSecret, nil
		})

		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "無效的 token"})
			c.Abort()
			return
		}

		// 檢查 token 是否有效
		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			// 將 member_id 存入上下文
			memberID, ok := claims["member_id"].(float64)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{"error": "無效的會員 ID"})
				c.Abort()
				return
			}
			c.Set("member_id", int(memberID))
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "無效的 token 內容"})
			c.Abort()
			return
		}

		// 繼續處理下一個處理函數
		c.Next()
	}
}

func Path(router *gin.RouterGroup) {
	// 版本控制
	v1 := router.Group("/v1")
	{
		// 測試路由
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(200, gin.H{"message": "pong"})
		})

		// 往後端傳資料不要用GET(蘇老師交代)
		// 會員路由
		members := v1.Group("/members")
		{
			// 公開路由：不需要 token 驗證
			members.POST("/register", handlers.RegisterMember) // 註冊會員
			members.POST("/login", handlers.LoginMember)       // 登入會員並獲取 token

			// 受保護路由：需要 token 驗證
			membersWithAuth := members.Group("")
			membersWithAuth.Use(AuthMiddleware())
			{
				membersWithAuth.GET("all", handlers.GetAllMembers)             // 查詢所有會員
				membersWithAuth.GET("/:id", handlers.GetMember)                // 查詢會員
				membersWithAuth.PUT("/:id", handlers.UpdateMember)             // 更新會員資料
				membersWithAuth.DELETE("/:id", handlers.DeleteMember)          // 刪除會員
				membersWithAuth.GET("/:id/history", handlers.GetMemberHistory) // 查詢特定會員的租賃歷史記錄
			}
		}

		// 車位路由
		parking := v1.Group("/parking")
		{
			// 公開路由：不需要 token 驗證
			parking.POST("share", handlers.ShareParkingSpot) // 共享車位

			// 受保護路由：需要 token 驗證
			parkingWithAuth := parking.Group("")
			parkingWithAuth.Use(AuthMiddleware())
			{
				parking.GET("/available", handlers.GetAvailableParkingSpots) // 查詢可用車位
				parkingWithAuth.GET("/:id", handlers.GetParkingSpot)         // 查詢特定車位
				parkingWithAuth.PUT("/:id", handlers.UpdateParkingSpot)      // 更新車位信息
			}
		}

		// 租用路由
		rent := v1.Group("/rent")
		{
			// 受保護路由：需要 token 驗證
			rentWithAuth := rent.Group("")
			rentWithAuth.Use(AuthMiddleware())
			{
				rentWithAuth.POST("", handlers.RentParkingSpot)       // 租用車位
				rentWithAuth.POST("/:id/leave", handlers.LeaveAndPay) // 離開結算
				rentWithAuth.GET("", handlers.GetRentRecords)         // 查詢所有租用紀錄
				rentWithAuth.GET("/:id", handlers.GetRentByID)        // 查詢特定租賃記錄
				rentWithAuth.DELETE("/:id", handlers.CancelRent)      // 取消租用
			}
		}
	}
}
