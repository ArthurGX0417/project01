package utils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/bcrypt"
)

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
	return err == nil
}

// getAESKey 動態加載 AES_KEY
func getAESKey() ([]byte, error) {
	aesKey := []byte(os.Getenv("AES_KEY"))
	if len(aesKey) == 0 {
		return nil, errors.New("AES_KEY environment variable is not set")
	}
	if len(aesKey) != 32 {
		return nil, errors.New("AES_KEY must be 32 bytes long")
	}
	return aesKey, nil
}

// EncryptPaymentInfo 使用 AES 加密 payment_info
func EncryptPaymentInfo(plainText string) (string, error) {
	aesKey, err := getAESKey()
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(aesKey)
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
	aesKey, err := getAESKey()
	if err != nil {
		return "", err
	}

	data, err := base64.StdEncoding.DecodeString(cipherText)
	if err != nil {
		return "", err
	}

	block, err := aes.NewCipher(aesKey)
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

// 檢查 AES_KEY 是否加載成功
func InitCrypto() error {
	aesKey := os.Getenv("AES_KEY")
	if aesKey == "" {
		return fmt.Errorf("AES_KEY environment variable is not set")
	}
	if len(aesKey) != 32 {
		return fmt.Errorf("AES_KEY must be 32 bytes long, got %d bytes", len(aesKey))
	}
	return nil
}
