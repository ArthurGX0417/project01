package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"os"

	"golang.org/x/crypto/bcrypt"
)

var JWTSecret []byte
var AESKey []byte // 新增全局 AES_KEY 變數

func InitJWTSecret() {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}
	if len(secret) < 32 {
		log.Fatal("JWT_SECRET must be at least 32 bytes long")
	}
	JWTSecret = []byte(secret)
	log.Printf("JWT_SECRET loaded successfully, length: %d", len(JWTSecret))
}

// HashPassword 使用 bcrypt 哈希密碼
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash password: %w", err)
	}
	return string(bytes), nil
}

// CheckPasswordHash 驗證密碼是否與哈希匹配
func CheckPasswordHash(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		log.Printf("Password verification failed: %v", err)
		return false
	}
	return true
}

// EncryptPaymentInfo 使用 AES 加密 payment_info
func EncryptPaymentInfo(plainText string) (string, error) {
	if AESKey == nil {
		return "", errors.New("AES_KEY not initialized")
	}

	block, err := aes.NewCipher(AESKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}

	cipherText := gcm.Seal(nonce, nonce, []byte(plainText), nil)
	return base64.StdEncoding.EncodeToString(cipherText), nil
}

// DecryptPaymentInfo 解密 payment_info
func DecryptPaymentInfo(cipherText string) (string, error) {
	if AESKey == nil {
		return "", errors.New("AES_KEY not initialized")
	}

	data, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(AESKey)
	if err != nil {
		return "", err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}

	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return "", errors.New("ciphertext too short")
	}

	nonce, cipherTextBytes := data[:nonceSize], data[nonceSize:]
	plainText, err := gcm.Open(nil, nonce, cipherTextBytes, nil)
	if err != nil {
		return "", err
	}

	return string(plainText), nil
}

// InitCrypto 初始化 AES_KEY 並儲存為全局變數
func InitCrypto() error {
	aesKey := os.Getenv("AES_KEY")
	if aesKey == "" {
		return fmt.Errorf("AES_KEY environment variable is not set")
	}
	if len(aesKey) != 32 {
		return fmt.Errorf("AES_KEY must be 32 bytes long, got %d bytes", len(aesKey))
	}
	AESKey = []byte(aesKey)
	log.Printf("AES_KEY loaded successfully, length: %d", len(AESKey))
	return nil
}

// IsEncrypted 檢查字符串是否為 AES 加密格式
func IsEncrypted(data string) bool {
	if data == "" || data == "NULL" {
		return true // 空值或 NULL 視為已處理
	}
	decoded, err := base64.StdEncoding.DecodeString(data)
	if err != nil {
		return false // 不是 Base64 編碼的數據
	}
	// AES-GCM 加密後數據至少包含 nonce (12 字節) + 密文
	return len(decoded) > 12
}
