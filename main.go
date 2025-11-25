package main

import (
	"log"
	"os"
	"project01/database"
	"project01/models"
	"project01/routes"
	"project01/utils"
	"strings"

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

	// 執行資料庫遷移
	database.DB.AutoMigrate(
		&models.Member{},
		&models.ParkingLot{},
		&models.Vehicle{},
		&models.Rent{},
	)
	log.Println("Database migration completed")

	// 確保預設管理員存在
	ensureAdminExists()

	// 檢查並更新現有密碼和 payment_info
	updatePasswordsAndPaymentInfo()

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

	// 啟動定時任務
	c := cron.New()

	// 移除預約超時檢查（因無預約需求）
	// 可保留空殼以供未來擴展
	c.Start()
	log.Println("Cron jobs started")

	// 啟動伺服器
	log.Println("Starting server on :8080")
	if err := r.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

// ensureAdminExists 檢查並創建預設管理員
func ensureAdminExists() {
	var admin models.Member
	// 檢查是否已經有 admin 角色
	if err := database.DB.Where("role = ?", "admin").First(&admin).Error; err == nil {
		log.Printf("Admin already exists: email=%s", admin.Email)
		return
	}

	// 哈希密碼
	hashedPassword, err := utils.HashPassword("adminj0j0") // 假設預設密碼
	if err != nil {
		log.Fatalf("Failed to hash admin password: %v", err)
	}
	encryptedPayment, err := utils.EncryptPaymentInfo("4758-1425-2536-5869") // 加密預設信用卡號
	if err != nil {
		log.Fatalf("Failed to encrypt payment info for admin: %v", err)
	}
	admin = models.Member{
		Email:       "adminj0j0@gmail.com",
		Phone:       "0936687137",
		Password:    hashedPassword,
		PaymentInfo: encryptedPayment,
		Role:        "admin",
		Name:        "adminj0j0",
	}
	// 插入資料庫
	if err := database.DB.Create(&admin).Error; err != nil {
		log.Fatalf("Failed to create default admin: %v", err)
	}

	log.Printf("Default admin created: email=%s", admin.Email)
}

// updatePasswordsAndPaymentInfo 檢查並更新現有密碼和 payment_info
func updatePasswordsAndPaymentInfo() {
	var members []models.Member
	if err := database.DB.Find(&members).Error; err != nil {
		log.Fatalf("Failed to fetch members: %v", err)
	}

	for _, member := range members {
		// 檢查密碼是否為明文
		if len(member.Password) != 60 || !strings.HasPrefix(member.Password, "$2a$") {
			log.Printf("Found plaintext password for member %s, updating...", member.Email)
			hashedPassword, err := utils.HashPassword(member.Password)
			if err != nil {
				log.Printf("Failed to hash password for member %s: %v", member.Email, err)
				continue
			}
			if err := database.DB.Model(&member).Update("password", hashedPassword).Error; err != nil {
				log.Printf("Failed to update password for member %s: %v", member.Email, err)
				continue
			}
			log.Printf("Updated password for member %s", member.Email)
		}

		// 檢查 payment_info 是否為明文或未加密
		if member.PaymentInfo != "" && !utils.IsEncrypted(member.PaymentInfo) {
			log.Printf("Found plaintext payment_info for member %s, updating...", member.Email)
			encryptedPayment, err := utils.EncryptPaymentInfo(member.PaymentInfo)
			if err != nil {
				log.Printf("Failed to encrypt payment_info for member %s: %v", member.Email, err)
				continue
			}
			if err := database.DB.Model(&member).Update("payment_info", encryptedPayment).Error; err != nil {
				log.Printf("Failed to update payment_info for member %s: %v", member.Email, err)
				continue
			}
			log.Printf("Updated payment_info for member %s", member.Email)
		}
	}
	log.Println("Password and payment_info update check completed")
}
