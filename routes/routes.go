package routes

import (
	"errors"
	"log"
	"net/http"
	"project01/handlers"
	"project01/utils"
	"strings"
	"time"

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
				"code":    "ERR_NO_AUTH_HEADER",
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
				"code":    "ERR_INVALID_AUTH_FORMAT",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]
		log.Printf("Parsing token: %s", tokenString)

		// 明確要求檢查 exp 字段
		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return utils.JWTSecret, nil
		}, jwt.WithExpirationRequired())

		// 添加詳細日誌
		if err != nil {
			log.Printf("Token parsing error: %v", err)
			if claims, ok := token.Claims.(jwt.MapClaims); ok {
				log.Printf("Token claims: exp=%v, current_time=%v", claims["exp"], time.Now().Unix())
			}
			if errors.Is(err, jwt.ErrTokenExpired) { // 使用 errors.Is 確保正確匹配
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "token 已過期",
					"error":   "Token has expired",
					"code":    "ERR_TOKEN_EXPIRED",
				})
			} else {
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "無效的 token",
					"error":   err.Error(),
					"code":    "ERR_INVALID_TOKEN",
				})
			}
			c.Abort()
			return
		}

		// 檢查 Claims 是否有效
		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
			// 確認 exp 字段存在
			if exp, ok := claims["exp"].(float64); !ok {
				log.Printf("Missing or invalid exp in token")
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "無效的 token 內容",
					"error":   "Missing or invalid exp claim",
					"code":    "ERR_INVALID_CLAIMS",
				})
				c.Abort()
				return
			} else {
				log.Printf("Token verified: exp=%v, current_time=%v", exp, time.Now().Unix())
			}

			// 確認 member_id 字段
			memberID, ok := claims["member_id"].(float64)
			if !ok {
				log.Printf("Missing or invalid member_id in token")
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "無效的會員 ID",
					"error":   "Invalid member_id in token",
					"code":    "ERR_INVALID_MEMBER_ID",
				})
				c.Abort()
				return
			}

			log.Printf("Token verified for member_id: %d", int(memberID))
			c.Set("member_id", int(memberID))
		} else {
			log.Printf("Invalid token claims or token is not valid")
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "無效的 token 內容",
				"error":   "Invalid token claims or token is not valid",
				"code":    "ERR_INVALID_CLAIMS",
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
