package routes

import (
	"log"
	"net/http"
	"project01/handlers"
	"project01/utils"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5" // 更新為 jwt/v5
)

// AuthMiddleware 驗證 JWT token
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "缺少 Authorization 標頭",
				"error":   "Authorization header is required",
			})
			c.Abort()
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "無效的 Authorization 格式",
				"error":   "Authorization header must be in the format 'Bearer <token>'",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// 添加日誌以檢查 token
		log.Printf("Parsing token: %s", tokenString)

		// 明確要求檢查 exp 字段
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return utils.JWTSecret, nil
		}, jwt.WithExpirationRequired())

		// 添加日誌以檢查錯誤
		if err != nil {
			log.Printf("Token parsing error: %v", err)
			if err == jwt.ErrTokenExpired {
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "token 已過期",
					"error":   "Token has expired",
				})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "無效的 token",
					"error":   err.Error(),
				})
			}
			c.Abort()
			return
		}

		// 添加日誌以檢查 token 是否有效
		log.Printf("Token is valid: %v", token.Valid)

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			memberID, ok := claims["member_id"].(float64)
			if !ok {
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "無效的會員 ID",
					"error":   "Invalid member_id in token",
				})
				c.Abort()
				return
			}
			c.Set("member_id", int(memberID))
		} else {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "無效的 token 內容",
				"error":   "Invalid token claims or token is not valid",
			})
			c.Abort()
			return
		}

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
				parkingWithAuth.GET("/available", handlers.GetAvailableParkingSpots) // 查詢可用車位
				parkingWithAuth.GET("/:id", handlers.GetParkingSpot)                 // 查詢特定車位
				parkingWithAuth.PUT("/:id", handlers.UpdateParkingSpot)              // 更新車位信息
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
