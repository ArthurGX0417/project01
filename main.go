package main

import (
	"log"
	"os"
	"project01/database"
	"project01/routes"
	"project01/services"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/robfig/cron/v3"
)

func main() {
	// 加載環境變量
	if err := godotenv.Load(); err != nil {
		log.Fatalf("Failed to load .env file: %v. Please ensure the .env file exists in the project root directory.", err)
	}
	log.Println(".env file loaded successfully")

	// 檢查 AES_KEY 是否加載成功
	aesKey := os.Getenv("AES_KEY")
	if aesKey == "" {
		log.Fatal("AES_KEY environment variable is not set after loading .env file. Please check the .env file content.")
	}
	if len(aesKey) != 32 {
		log.Fatalf("AES_KEY must be 32 bytes long, got %d bytes", len(aesKey))
	}
	log.Printf("AES_KEY loaded successfully (length: %d bytes)", len(aesKey))

	// 初始化資料庫
	database.InitDB()

	// 設置 Gin 模式為 release
	gin.SetMode(gin.ReleaseMode)

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
