package database

import (
    "log"
    "os"
    "time"

    "gorm.io/driver/mysql"
    "gorm.io/gorm"
    "gorm.io/gorm/logger"
)

var DB *gorm.DB

func InitDB() {
    // 根據環境設置日誌級別
    logLevel := logger.Info
    if os.Getenv("GIN_MODE") == "release" {
        logLevel = logger.Warn // 生產環境減少日誌
    }

    // 資料庫連線參數
    dsn := "parking_user:parking1234@tcp(127.0.0.1:3306)/parking_db?charset=utf8mb4&parseTime=True&loc=Local"
    var err error

    // 重試機制
    maxRetries := 5
    retryInterval := 5 * time.Second
    for i := 0; i < maxRetries; i++ {
        DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{
            Logger: logger.Default.LogMode(logLevel),
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

    log.Println("Database initialized successfully with GORM")
}
