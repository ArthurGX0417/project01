package database

import (
	"fmt"
	"log"
	"os"
	"project01/models"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
)

var DB *gorm.DB

func InitDB() {
	// 根據環境設置日誌級別
	logLevel := logger.Info
	if os.Getenv("GIN_MODE") == "release" {
		logLevel = logger.Warn // 生產環境減少日誌
	}

	// 從環境變量中讀取資料庫連線參數
	user := os.Getenv("DB_USER")
	password := os.Getenv("DB_PASSWORD")
	host := os.Getenv("DB_HOST")
	port := os.Getenv("DB_PORT")
	name := os.Getenv("DB_NAME")

	// 檢查是否有缺少的環境變量
	if user == "" || password == "" || host == "" || port == "" || name == "" {
		log.Fatalf("Missing required database environment variables. Ensure DB_USER, DB_PASSWORD, DB_HOST, DB_PORT, and DB_NAME are set in the .env file.")
	}

	// 動態構建 DSN，設置 loc=Asia/Shanghai
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Asia%%2FShanghai", user, password, host, port, name)
	var err error

	// 重試機制
	maxRetries := 5
	retryInterval := 5 * time.Second
	for i := 0; i < maxRetries; i++ {
		DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
			Logger: logger.Default.LogMode(logLevel),
			NamingStrategy: schema.NamingStrategy{
				SingularTable: true, // 設置為 true，確保使用單數表名
			},
			DisableForeignKeyConstraintWhenMigrating: true, // 禁用外鍵約束檢查
		})
		if err == nil {
			break
		}
		log.Printf("Failed to connect to database (attempt %d/%d): %v", i+1, maxRetries, err)
		if i < maxRetries-1 {
			time.Sleep(retryInterval)
		}
	}
	if err != nil {
		log.Fatalf("Failed to open database after %d attempts: %v", maxRetries, err)
	}

	// 設置連線池
	sqlDB, err := DB.DB()
	if err != nil {
		log.Fatalf("Failed to get sql.DB: %v", err)
	}

	// 連線池配置
	sqlDB.SetMaxIdleConns(10)           // 最大閒置連線數
	sqlDB.SetMaxOpenConns(100)          // 最大開啟連線數
	sqlDB.SetConnMaxLifetime(time.Hour) // 連線最大存活時間

	// 檢查連線
	if err = sqlDB.Ping(); err != nil {
		log.Fatalf("Failed to ping database: %v", err)
	}

	// 確認當前資料庫
	var dbName string
	if err := DB.Raw("SELECT DATABASE()").Scan(&dbName).Error; err != nil {
		log.Fatalf("Failed to get current database: %v", err)
	}
	log.Printf("Connected to database: %s", dbName)

	// 自定義遷移邏輯，避免修改現有欄位
	migrator := DB.Migrator()
	modelsToMigrate := []interface{}{
		&models.Member{},
		&models.ParkingSpot{},
		&models.ParkingSpotAvailableDay{},
		&models.Rent{},
	}

	for _, model := range modelsToMigrate {
		// 檢查表是否存在
		if !migrator.HasTable(model) {
			// 表不存在，創建新表
			if err := migrator.CreateTable(model); err != nil {
				log.Fatalf("Failed to create table for model %T: %v", model, err)
			}
		} else {
			// 表存在，只添加缺少的欄位，不修改現有欄位
			if err := migrator.AutoMigrate(model); err != nil {
				log.Fatalf("Failed to auto-migrate model %T: %v", model, err)
			}
		}
	}

	log.Println("Database initialized successfully with GORM")
}
