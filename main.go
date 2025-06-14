package main

import (
	"log"
	"os"
	"project01/database"
	"project01/models"
	"project01/routes"
	"project01/services"
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
		&models.ParkingSpot{},
		&models.Rent{},
		&models.ParkingSpotAvailableDay{},
	)
	log.Println("Database migration completed")

	// 同步 parking spot 狀態
	if err := services.SyncParkingSpotStatus(); err != nil {
		log.Fatalf("Failed to sync parking spot statuses: %v", err)
	}
	log.Println("Parking spot statuses synced successfully")

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

	// 檢查預約超時定時任務（每 5 分鐘執行一次）
	_, err := c.AddFunc("*/5 * * * *", func() {
		log.Println("Checking for expired reservations...")
		if err := services.CheckExpiredReservations(); err != nil {
			log.Printf("Failed to check expired reservations: %v", err)
		}
	})
	if err != nil {
		log.Fatalf("Failed to schedule expired reservations check cron job: %v", err)
	}

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
	hashedPassword, err := utils.HashPassword("default_password") // 假設預設密碼
	if err != nil {
		log.Fatalf("Failed to hash admin password: %v", err)
	}
	encryptedPayment, err := utils.EncryptPaymentInfo("9999-9595-9292-9090") // 加密預設信用卡號
	if err != nil {
		log.Fatalf("Failed to encrypt payment info for admin: %v", err)
	}
	admin = models.Member{
		MemberID:      1,
		Name:          "adminjojo",
		Email:         "adminjojo@gmail.com",
		Phone:         "0936687137",
		Password:      hashedPassword,
		Role:          "admin",
		PaymentMethod: "credit_card",
		PaymentInfo:   encryptedPayment,
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
