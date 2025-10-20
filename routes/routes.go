package routes

import (
	"errors"
	"log"
	"net/http"
	"project01/database"
	"project01/handlers"
	"project01/models"
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

		token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return utils.JWTSecret, nil
		}, jwt.WithExpirationRequired())

		if err != nil {
			log.Printf("Token parsing error: %v", err)
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

		if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
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

			role, ok := claims["role"].(string)
			if !ok || (role != "renter" && role != "admin") { // 移除 shared_owner
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
			c.Set("role", role)
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

// 在routes.go的AuthMiddleware()和RoleMiddleware()後添加
func LicenseMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		memberID, exists := c.Get("member_id")
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
		memberIDInt, ok := memberID.(int)
		if !ok {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "未授權",
				"error":   "invalid member_id type",
				"code":    "ERR_INVALID_MEMBER_ID_TYPE",
			})
			c.Abort()
			return
		}

		var member models.Member
		if err := database.DB.First(&member, memberIDInt).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"status":  false,
				"message": "無法獲取會員資訊",
				"error":   err.Error(),
				"code":    "ERR_MEMBER_NOT_FOUND",
			})
			c.Abort()
			return
		}
		if member.LicensePlate == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"status":  false,
				"message": "會員無車牌資訊",
				"error":   "license_plate is empty",
				"code":    "ERR_NO_LICENSE_PLATE",
			})
			c.Abort()
			return
		}
		c.Set("license_plate", member.LicensePlate)
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

		// 權限檢查：僅 admin 或自己可查詢
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
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
		})

		// 會員路由
		members := v1.Group("/members")
		{
			// 公開路由：不需要 token 驗證
			members.POST("/register", handlers.RegisterMember) //註冊
			members.POST("/login", handlers.LoginMember)       //登入

			// 受保護路由：需要 token 驗證
			membersWithAuth := members.Group("")
			membersWithAuth.Use(AuthMiddleware())
			{
				membersWithAuth.GET("/profile", RoleMiddleware("renter"), handlers.GetMemberProfile)              //查看個人資料
				membersWithAuth.GET("/all", RoleMiddleware("admin"), handlers.GetAllMembers)                      //查詢所有會員
				membersWithAuth.GET("/:id", RoleMiddleware("admin"), handlers.GetMember)                          //查詢特定會員
				membersWithAuth.GET("/:id/history", MemberRentHistoryMiddleware(), handlers.GetMemberRentHistory) //查詢特定會員的租賃記錄
				membersWithAuth.PUT("/:id", RoleMiddleware("admin"), handlers.UpdateMember)                       //更新特定會員的資訊
				membersWithAuth.PUT("/:id/license-plate", RoleMiddleware("renter"), handlers.UpdateLicensePlate)  //更新車牌號碼
				membersWithAuth.DELETE("/:id", RoleMiddleware("admin"), handlers.DeleteMember)                    //刪除特定會員
			}
		}

		// 車位路由
		parking := v1.Group("/parking")
		{
			parkingWithAuth := parking.Group("")
			parkingWithAuth.Use(AuthMiddleware())
			{
				parkingWithAuth.POST("", RoleMiddleware("admin"), handlers.CreateParkingLot)                           // 新增停車場
				parkingWithAuth.GET("/available", RoleMiddleware("renter", "admin"), handlers.GetAvailableParkingLots) // 查詢可用停車場
				parkingWithAuth.GET("/:id", RoleMiddleware("renter", "admin"), handlers.GetParkingLot)                 // 查詢特定停車場詳情 (剩餘位子、經緯度)
				parkingWithAuth.PUT("/:id", RoleMiddleware("admin"), handlers.UpdateParkingLot)                        // 更新停車場
				parkingWithAuth.DELETE("/:id", RoleMiddleware("admin"), handlers.DeleteParkingLot)                     // 刪除停車場
			}
		}

		// 租用路由
		rent := v1.Group("/rent")
		{
			rentWithAuth := rent.Group("")
			rentWithAuth.Use(AuthMiddleware(), RoleMiddleware("renter"), LicenseMiddleware()) // 添加LicenseMiddleware，只限renter路由
			{
				rentWithAuth.POST("", RoleMiddleware("renter"), handlers.EnterParkingSpot)                                  //租用車位（車牌掃描輸入）
				rentWithAuth.POST("/leave", RoleMiddleware("renter"), handlers.LeaveParkingSpot)                            //離開結算（車牌掃描輸入）
				rentWithAuth.POST("/:id/notify", RoleMiddleware("renter"), handlers.GenerateParkingNotification)            //停車結算通知
				rentWithAuth.GET("/currently-rented", RoleMiddleware("renter"), handlers.GetCurrentlyRentedSpots)           //查詢當前租用的車位
				rentWithAuth.GET("", RoleMiddleware("renter"), handlers.GetRentRecordsByLicensePlate)                       //查詢租用紀錄
				rentWithAuth.GET("/:id", RoleMiddleware("renter"), handlers.GetRentByID)                                    //查詢特定租賃記錄
				rentWithAuth.GET("/total-cost", RoleMiddleware("renter"), handlers.GetTotalCostByLicensePlate)              //查詢總費用
				rentWithAuth.GET("/availability/:id", RoleMiddleware("renter", "admin"), handlers.CheckParkingAvailability) //查詢特定停車場可用位子
			}
		}
	}
}
