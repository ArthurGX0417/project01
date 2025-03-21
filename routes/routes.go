package routes

import (
	"project01/handlers"

	"github.com/gin-gonic/gin"
)

func Path(router *gin.RouterGroup) {
	// 測試路由
	router.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "pong"})
	})

	// 往後端傳資料不要用GET(蘇老師交代)
	// 會員相關路由
	router.POST("/member/register", handlers.RegisterMember)     //註冊會員
	router.POST("/member/login", handlers.LoginMember)           //登入會員
	router.GET("/member/:id", handlers.GetMember)                //查詢會員
	router.GET("/member", handlers.GetAllMembers)                //查詢所有會員
	router.PUT("/member/:id", handlers.UpdateMember)             //更新會員資料
	router.DELETE("/member/:id", handlers.DeleteMember)          //刪除會員
	router.GET("/member/:id/history", handlers.GetMemberHistory) //查詢特定會員的租賃歷史記錄

	// 車位相關路由
	router.POST("/parking/share", handlers.ShareParkingSpot)            //共享車位
	router.GET("/parking/available", handlers.GetAvailableParkingSpots) //查詢可用車位
	router.GET("/parking/:id", handlers.GetParkingSpot)                 //查詢特定車位
	router.PUT("/parking/:id", handlers.UpdateParkingSpot)              //更新車位信息

	// 租用相關路由
	router.POST("/rent/rent", handlers.RentParkingSpot)  //租用車位
	router.POST("/rent/:id/leave", handlers.LeaveAndPay) //離開結算
	router.GET("/rent", handlers.GetRentRecords)         //查詢所有租用紀錄
	router.GET("/rent/:id", handlers.GetRentByID)        //查詢特定租賃記錄
	router.DELETE("/rent/:id", handlers.CancelRent)      //取消租用
}
