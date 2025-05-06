package routes

import (
	"errors"
	"log"
	"net/http"
	"project01/handlers"
	"project01/utils"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// AuthMiddleware 驗證 JWT token，並提取 member_id 和 role
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
			if errors.Is(err, jwt.ErrTokenExpired) {
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

			// 確認 role 字段
			role, ok := claims["role"].(string)
			if !ok || (role != "renter" && role != "shared_owner" && role != "admin") {
				log.Printf("Missing or invalid role in token: %v", role)
				c.JSON(http.StatusUnauthorized, gin.H{
					"status":  false,
					"message": "無效的角色",
					"error":   "Invalid role in token",
					"code":    "ERR_INVALID_ROLE",
				})
				c.Abort()
				return
			}

			log.Printf("Token verified for member_id: %d, role: %s", int(memberID), role)
			c.Set("member_id", int(memberID))
			c.Set("role", role) // 將 role 存入上下文
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

// RoleMiddleware 檢查會員角色是否符合要求
func RoleMiddleware(allowedRoles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "無法獲取角色資訊",
				"error":   "Role not found in context",
				"code":    "ERR_ROLE_NOT_FOUND",
			})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "無效的角色類型",
				"error":   "Invalid role type",
				"code":    "ERR_INVALID_ROLE_TYPE",
			})
			c.Abort()
			return
		}

		// 允許 admin 角色訪問所有端點
		if roleStr == "admin" {
			c.Next()
			return
		}

		allowed := false
		for _, allowedRole := range allowedRoles {
			if roleStr == allowedRole {
				allowed = true
				break
			}
		}

		if !allowed {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  false,
				"message": "權限不足",
				"error":   "Insufficient role permissions",
				"code":    "ERR_INSUFFICIENT_PERMISSIONS",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// MemberRentHistoryMiddleware 檢查會員是否有權訪問租賃歷史
func MemberRentHistoryMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		currentMemberID, exists := c.Get("member_id")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "未授權",
				"error":   "member_id not found in token",
				"code":    "ERR_NO_MEMBER_ID",
			})
			c.Abort()
			return
		}

		currentMemberIDInt, ok := currentMemberID.(int)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "未授權",
				"error":   "invalid member_id type",
				"code":    "ERR_INVALID_MEMBER_ID",
			})
			c.Abort()
			return
		}

		role, exists := c.Get("role")
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "未授權",
				"error":   "role not found in token",
				"code":    "ERR_NO_ROLE",
			})
			c.Abort()
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "未授權",
				"error":   "invalid role type",
				"code":    "ERR_INVALID_ROLE",
			})
			c.Abort()
			return
		}

		requestedMemberIDStr := c.Param("id")
		requestedMemberID, err := strconv.Atoi(requestedMemberIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "無效的會員 ID",
				"error":   err.Error(),
				"code":    "ERR_INVALID_ID",
			})
			c.Abort()
			return
		}

		// 權限檢查
		if roleStr != "admin" && currentMemberIDInt != requestedMemberID {
			c.JSON(http.StatusForbidden, gin.H{
				"status":  false,
				"message": "無權限",
				"error":   "you can only view your own rent history",
				"code":    "ERR_INSUFFICIENT_PERMISSIONS",
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
				// 查看個人資料：任何已認證的用戶都可以訪問
				membersWithAuth.GET("/profile", handlers.GetMemberProfile)
				// 管理員專屬路由
				membersWithAuth.GET("/all", RoleMiddleware("admin"), handlers.GetAllMembers)                      // 查詢所有會員
				membersWithAuth.GET("/:id", RoleMiddleware("admin"), handlers.GetMember)                          // 查詢特定會員
				membersWithAuth.GET("/:id/history", MemberRentHistoryMiddleware(), handlers.GetMemberRentHistory) // 查詢特定會員的租賃歷史記錄
				membersWithAuth.PUT("/:id", RoleMiddleware("admin"), handlers.UpdateMember)                       // 更新會員資料
				membersWithAuth.DELETE("/:id", RoleMiddleware("admin"), handlers.DeleteMember)                    // 刪除會員
			}
		}

		// 車位路由
		parking := v1.Group("/parking")
		{
			// 公開路由：不需要 token 驗證
			// 根據需求，共享車位需要驗證並限制為 shared_owner 或 admin
			parkingWithAuth := parking.Group("")
			parkingWithAuth.Use(AuthMiddleware())
			{
				// 共享車位：僅 shared_owner 和 admin 可以操作
				parkingWithAuth.POST("/share", RoleMiddleware("shared_owner", "admin"), handlers.ShareParkingSpot)
				// 查詢可用車位：renter 和 shared_owner 都可以訪問
				parkingWithAuth.GET("/available", RoleMiddleware("renter", "shared_owner", "admin"), handlers.GetAvailableParkingSpots)
				// 查詢特定車位：renter 和 shared_owner 都可以訪問
				parkingWithAuth.GET("/:id", RoleMiddleware("renter", "shared_owner"), handlers.GetParkingSpot)
				// 查看車位收入：shared_owner 和 admin 都可以訪問
				parkingWithAuth.GET("/:id/income", RoleMiddleware("shared_owner", "admin"), handlers.GetParkingSpotIncome)
				// 更新車位：僅 shared_owner 和 admin 可以操作
				parkingWithAuth.PUT("/:id", RoleMiddleware("shared_owner", "admin"), handlers.UpdateParkingSpot)
				// 刪除車位：僅 shared_owner 和 admin 可以操作
				parkingWithAuth.DELETE("/:id", RoleMiddleware("shared_owner", "admin"), handlers.DeleteParkingSpot)
			}
		}

		// 租用路由
		rent := v1.Group("/rent")
		{
			// 受保護路由：需要 token 驗證
			rentWithAuth := rent.Group("")
			rentWithAuth.Use(AuthMiddleware())
			{
				// 租用車位：renter 和 shared_owner 都可以操作
				rentWithAuth.POST("", RoleMiddleware("renter", "shared_owner"), handlers.RentParkingSpot)
				// 預約車位：僅 renter 可以操作
				rentWithAuth.POST("/reserve", RoleMiddleware("renter"), handlers.ReserveParkingSpot)
				// 確認預約：僅 renter 可以操作
				rentWithAuth.POST("/:id/confirm", RoleMiddleware("renter"), handlers.ConfirmReservation)
				// 離開結算：renter 和 shared_owner 都可以操作
				rentWithAuth.POST("/:id/leave", RoleMiddleware("renter", "shared_owner"), handlers.LeaveAndPay)
				// 查詢所有租賃記錄：renter 和 shared_owner 都可以訪問
				rentWithAuth.GET("", RoleMiddleware("renter", "shared_owner"), handlers.GetRentRecords)
				// 查詢特定租賃記錄：renter 和 shared_owner 都可以訪問
				rentWithAuth.GET("/:id", RoleMiddleware("renter", "shared_owner"), handlers.GetRentByID)
				// 查詢所有預約記錄：renter 和 shared_owner 都可以訪問
				rentWithAuth.GET("/reservations", RoleMiddleware("renter", "shared_owner"), handlers.GetReservations)
				// 取消租用：renter 和 shared_owner 都可以操作
				rentWithAuth.DELETE("/:id", RoleMiddleware("renter", "shared_owner"), handlers.CancelRent)

			}
		}
	}
}
