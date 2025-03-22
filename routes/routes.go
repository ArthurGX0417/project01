package routes

import (
	"project01/handlers"

	"github.com/gin-gonic/gin"
)

// AuthMiddleware 是驗證中間件的佔位符
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement authentication logic (e.g., check JWT token)
		// For now, just proceed to the next handler
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
			members.POST("/register", handlers.RegisterMember) // 註冊會員
			members.POST("/login", handlers.LoginMember)       // 登入會員
			// Protected routes with AuthMiddleware
			membersWithAuth := members.Group("")
			membersWithAuth.Use(AuthMiddleware())
			{
				membersWithAuth.GET("", handlers.GetAllMembers)                // 查詢所有會員
				membersWithAuth.GET("/:id", handlers.GetMember)                // 查詢會員
				membersWithAuth.PUT("/:id", handlers.UpdateMember)             // 更新會員資料
				membersWithAuth.DELETE("/:id", handlers.DeleteMember)          // 刪除會員
				membersWithAuth.GET("/:id/history", handlers.GetMemberHistory) // 查詢特定會員的租賃歷史記錄
			}
		}

		// 車位路由
		parking := v1.Group("/parking")
		{
			parking.POST("", handlers.ShareParkingSpot)                  // 共享車位
			parking.GET("/available", handlers.GetAvailableParkingSpots) // 查詢可用車位
			// Protected routes with AuthMiddleware
			parkingWithAuth := parking.Group("")
			parkingWithAuth.Use(AuthMiddleware())
			{
				parkingWithAuth.GET("/:id", handlers.GetParkingSpot)    // 查詢特定車位
				parkingWithAuth.PUT("/:id", handlers.UpdateParkingSpot) // 更新車位信息
			}
		}

		// 租用路由
		rent := v1.Group("/rent")
		{
			// Protected routes with AuthMiddleware
			rentWithAuth := rent.Group("")
			rentWithAuth.Use(AuthMiddleware())
			{
				rentWithAuth.POST("", handlers.RentParkingSpot)        // 租用車位
				rentWithAuth.POST("/:id/settle", handlers.LeaveAndPay) // 離開結算 (renamed from /leave)
				rentWithAuth.GET("", handlers.GetRentRecords)          // 查詢所有租用紀錄
				rentWithAuth.GET("/:id", handlers.GetRentByID)         // 查詢特定租賃記錄
				rentWithAuth.DELETE("/:id", handlers.CancelRent)       // 取消租用
			}
		}
	}
}
