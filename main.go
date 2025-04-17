package main

import (
	"log"
	"os"
	"project01/database"
	"project01/routes"
	"project01/services"
	"project01/utils"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	// 載入 .env 檔案
	if err := godotenv.Load(); err != nil {
		log.Printf("No .env file found, using default environment variables: %v", err)
	}

	// 調用 AES_KEY 是否加載成功
	if err := utils.InitCrypto(); err != nil {
		log.Fatalf("Failed to initialize crypto: %v", err)
	}
	log.Println("Crypto initialized successfully")

	// 初始化 JWTSecret
	utils.InitJWTSecret()

	// 初始化資料庫
	database.InitDB()

	// 設置 Gin 模式為 release
	gin.SetMode(gin.ReleaseMode)
	ginMode := os.Getenv("GIN_MODE")
	if ginMode == "" {
		ginMode = gin.ReleaseMode
	}
	gin.SetMode(ginMode)
	log.Printf("Gin mode set to %s", ginMode)

	// 初始化 Gin 路由器
	r := gin.Default()

	// 創建一個 API 路由組
	api := r.Group("/api")
	{
		routes.Path(api)
	}

	// 啟動每月結算定時任務
	c := cron.New()
	_, err := c.AddFunc("0 0 1 * *", func() {
		log.Println("Running monthly settlement...")
		if err := services.MonthlySettlement(); err != nil {
			log.Printf("Monthly settlement failed: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Failed to schedule cron job: %v", err)
	}
	c.Start()

	// 啟動伺服器
	log.Println("Starting server on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
