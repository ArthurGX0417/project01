package services

import (
	"errors"
	"fmt"
	"log"
	"math"
	"project01/database"
	"project01/models"
	"time"

	"gorm.io/gorm"
)

// CalculateRentCost 統一計算租賃費用
func CalculateRentCost(rent models.Rent, spot models.ParkingSpot, actualEndTime time.Time) (float64, error) {
	// 確保所有時間為 CST
	cstZone := time.FixedZone("CST", 8*60*60)
	rent.StartTime = rent.StartTime.In(cstZone)
	rent.EndTime = rent.EndTime.In(cstZone)
	actualEndTime = actualEndTime.In(cstZone)

	if actualEndTime.Before(rent.StartTime) {
		log.Printf("actual_end_time %v is before start_time %v for rent_id %d", actualEndTime, rent.StartTime, rent.RentID)
		return 0, fmt.Errorf("actual end time %v cannot be earlier than start time %v", actualEndTime, rent.StartTime)
	}

	if spot.PricePerHalfHour <= 0 || spot.DailyMaxPrice <= 0 {
		return 0, fmt.Errorf("invalid pricing for spot_id %d: PricePerHalfHour=%.2f, DailyMaxPrice=%.2f", spot.SpotID, spot.PricePerHalfHour, spot.DailyMaxPrice)
	}

	durationMinutes := actualEndTime.Sub(rent.StartTime).Minutes()
	durationDays := durationMinutes / (24 * 60)

	var totalCost float64
	if durationMinutes <= 5 {
		totalCost = 0
	} else {
		halfHours := math.Floor(durationMinutes / 30)
		remainingMinutes := durationMinutes - (halfHours * 30)
		if remainingMinutes > 5 {
			halfHours++
		}
		totalCost = halfHours * spot.PricePerHalfHour

		if spot.DailyMaxPrice > 0 {
			days := math.Ceil(durationDays)
			maxCost := spot.DailyMaxPrice * days
			totalCost = math.Min(totalCost, maxCost)
		}

		overtimeMinutes := actualEndTime.Sub(rent.EndTime).Minutes()
		if overtimeMinutes > 0 {
			overtimeHalfHours := math.Ceil(overtimeMinutes / 30.0)
			overtimeCost := overtimeHalfHours * (spot.PricePerHalfHour * 2)
			totalCost += overtimeCost
		}

		if totalCost > spot.DailyMaxPrice*30 {
			log.Printf("Abnormal total_cost %.2f for rent_id %d, capping at %.2f", totalCost, rent.RentID, spot.DailyMaxPrice*30)
			totalCost = spot.DailyMaxPrice * 30
		}
	}

	return totalCost, nil
}

// RentParkingSpot 租用車位
func RentParkingSpot(rent *models.Rent) error {
	// 確保時間為 CST
	cstZone := time.FixedZone("CST", 8*60*60)
	rent.StartTime = rent.StartTime.In(cstZone)
	rent.EndTime = rent.EndTime.In(cstZone)

	// 驗證 member_id
	var member models.Member
	if err := database.DB.Where("member_id = ?", rent.MemberID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("member with ID %d not found", rent.MemberID)
		}
		log.Printf("Failed to verify member: member_id=%d, error=%v", rent.MemberID, err)
		return fmt.Errorf("failed to verify member: %w", err)
	}

	// 移除 FetchAvailableDays 檢查，假設車位可用性由其他邏輯保證
	if rent.StartTime.After(rent.EndTime) {
		return fmt.Errorf("start_time cannot be later than end_time")
	}

	// 設置 actual_end_time 為 NULL
	rent.ActualEndTime = nil
	// 插入租用記錄
	if err := database.DB.Create(rent).Error; err != nil {
		log.Printf("Failed to rent parking spot: spot_id=%d, error=%v", rent.SpotID, err)
		return fmt.Errorf("failed to rent parking spot: %w", err)
	}

	log.Printf("Successfully rented parking spot: spot_id=%d, rent_id=%d", rent.SpotID, rent.RentID)
	return nil
}

// GetRentRecords 查詢租用紀錄
func GetRentRecords() ([]models.Rent, error) {
	var rents []models.Rent
	// 查詢所有租用記錄
	if err := database.DB.Find(&rents).Error; err != nil {
		log.Printf("Failed to query rent records: error=%v", err)
		return nil, fmt.Errorf("failed to query rent records: %w", err)
	}
	log.Printf("Successfully fetched %d rent records", len(rents))
	return rents, nil
}

// CancelRent 取消租用
func CancelRent(id int) error {
	var rent models.Rent

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred: rent_id=%d, error=%v", id, r)
		}
	}()

	// 檢查租用是否存在
	if err := tx.First(&rent, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Rent not found: rent_id=%d", id)
			tx.Rollback()
			return fmt.Errorf("rent with ID %d not found", id)
		}
		tx.Rollback()
		log.Printf("Failed to query rent: rent_id=%d, error=%v", id, err)
		return fmt.Errorf("failed to query rent: %w", err)
	}

	// 檢查租賃狀態
	if rent.ActualEndTime != nil {
		tx.Rollback()
		log.Printf("Cannot cancel already settled rent: rent_id=%d", id)
		return fmt.Errorf("rent with ID %d has already been settled", id)
	}
	if rent.Status != "pending" && rent.Status != "reserved" {
		tx.Rollback()
		log.Printf("Cannot cancel rent with invalid status: rent_id=%d, status=%s", id, rent.Status)
		return fmt.Errorf("rent with ID %d has invalid status: %s", id, rent.Status)
	}

	// 更新租賃狀態為 canceled
	rent.Status = "canceled"
	if err := tx.Save(&rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to cancel rent: rent_id=%d, error=%v", id, err)
		return fmt.Errorf("failed to cancel rent: %w", err)
	}

	// 更新車位狀態
	var spot models.ParkingSpot
	if err := tx.Where("spot_id = ?", rent.SpotID).First(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to query parking spot: spot_id=%d, error=%v", rent.SpotID, err)
		return fmt.Errorf("failed to query parking spot: %w", err)
	}

	cstZone := time.FixedZone("CST", 8*60*60)
	newStatus, err := UpdateParkingSpotStatus(tx, rent.SpotID, time.Now().In(cstZone), cstZone)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", rent.SpotID, err)
		return fmt.Errorf("failed to update parking spot status: %w", err)
	}
	spot.Status = newStatus
	if err := tx.Save(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status in DB: spot_id=%d, error=%v", spot.SpotID, err)
		return fmt.Errorf("failed to update parking spot status: %w", err)
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction: rent_id=%d, error=%v", id, err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully canceled rent: rent_id=%d", id)
	return nil
}

// LeaveAndPay 離開付款
func LeaveAndPay(rentID int, actualEndTime time.Time) (float64, error) {
	var rent models.Rent
	var spot models.ParkingSpot

	// 確保 actualEndTime 是 CST 時區
	cstZone := time.FixedZone("CST", 8*60*60)
	actualEndTime = actualEndTime.In(cstZone)

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred: rent_id=%d, error=%v", rentID, r)
		}
	}()

	// 查詢租用記錄和車位資訊
	if err := tx.First(&rent, rentID).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to query rent: rent_id=%d, error=%v", rentID, err)
		return 0, fmt.Errorf("failed to query rent: %w", err)
	}
	if err := tx.Where("spot_id = ?", rent.SpotID).First(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to query parking spot: spot_id=%d, error=%v", rent.SpotID, err)
		return 0, fmt.Errorf("failed to query parking spot: %w", err)
	}

	// 驗證 spot_id 一致性
	if spot.SpotID != rent.SpotID {
		tx.Rollback()
		log.Printf("Inconsistent spot ID: rent.SpotID=%d, spot.SpotID=%d", rent.SpotID, spot.SpotID)
		return 0, fmt.Errorf("inconsistent spot ID: expected %d, got %d", rent.SpotID, spot.SpotID)
	}

	// 計算費用
	totalCost, err := CalculateRentCost(rent, spot, actualEndTime)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to calculate rent cost: rent_id=%d, error=%v", rentID, err)
		return 0, fmt.Errorf("failed to calculate rent cost: %w", err)
	}

	// 更新租用記錄
	rent.ActualEndTime = &actualEndTime
	rent.TotalCost = totalCost
	rent.Status = "completed"
	if err := tx.Save(&rent).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update rent: rent_id=%d, error=%v", rentID, err)
		return 0, fmt.Errorf("failed to update rent: %w", err)
	}

	// 更新車位狀態
	newStatus, err := UpdateParkingSpotStatus(tx, rent.SpotID, actualEndTime, cstZone)
	if err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", rent.SpotID, err)
		return 0, fmt.Errorf("failed to update parking spot status: %w", err)
	}
	spot.Status = newStatus
	if err := tx.Save(&spot).Error; err != nil {
		tx.Rollback()
		log.Printf("Failed to update parking spot status in DB: spot_id=%d, error=%v", spot.SpotID, err)
		return 0, fmt.Errorf("failed to update parking spot status: %w", err)
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction: rent_id=%d, error=%v", rentID, err)
		return 0, fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully processed leave and pay: rent_id=%d, total_cost=%.2f", rentID, totalCost)
	return totalCost, nil
}

// GetRentByID 查詢特定租賃記錄
func GetRentByID(id int, memberID int, role string) (*models.Rent, []models.ParkingSpotAvailableDay, error) {
	var rent models.Rent

	// 查詢租賃記錄
	if err := database.DB.
		Preload("Member").
		Preload("ParkingSpot").
		Preload("ParkingSpot.Member").
		First(&rent, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Rent not found: rent_id=%d", id)
			return nil, nil, fmt.Errorf("rent with ID %d not found", id)
		}
		log.Printf("Failed to query rent: rent_id=%d, error=%v", id, err)
		return nil, nil, fmt.Errorf("failed to query rent: %w", err)
	}

	// 權限檢查：非管理員只能查看自己的租賃記錄
	if role != "admin" && rent.MemberID != memberID {
		log.Printf("Unauthorized access to rent: rent_id=%d, member_id=%d, requesting_member_id=%d", id, rent.MemberID, memberID)
		return nil, nil, fmt.Errorf("unauthorized access to rent with ID %d", id)
	}

	// 移除 FetchAvailableDays，改為空切片
	availableDays := make([]models.ParkingSpotAvailableDay, 0)

	log.Printf("Successfully fetched rent: rent_id=%d, member_id=%d", id, memberID)
	return &rent, availableDays, nil
}

// ReserveParkingSpot 創建車位預約記錄
func ReserveParkingSpot(reservation *models.Rent) error {
	// 確保時間為 CST
	cstZone := time.FixedZone("CST", 8*60*60)
	reservation.StartTime = reservation.StartTime.In(cstZone)
	reservation.EndTime = reservation.EndTime.In(cstZone)

	// 驗證會員是否存在
	var member models.Member
	if err := database.DB.Where("member_id = ?", reservation.MemberID).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return fmt.Errorf("member with ID %d not found", reservation.MemberID)
		}
		log.Printf("Failed to verify member: member_id=%d, error=%v", reservation.MemberID, err)
		return fmt.Errorf("failed to verify member: %w", err)
	}

	// 檢查車位是否存在
	var spot models.ParkingSpot
	if err := database.DB.Where("spot_id = ?", reservation.SpotID).First(&spot).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			log.Printf("Parking spot not found: spot_id=%d", reservation.SpotID)
			return fmt.Errorf("parking spot %d not found", reservation.SpotID)
		}
		log.Printf("Failed to query parking spot: spot_id=%d, error=%v", reservation.SpotID, err)
		return fmt.Errorf("failed to query parking spot: %w", err)
	}

	// 移除 FetchAvailableDays 檢查，假設車位可用性由其他邏輯保證
	if reservation.StartTime.After(reservation.EndTime) {
		return fmt.Errorf("start_time cannot be later than end_time")
	}

	// 設置 ActualEndTime 為 NULL，並確保 Status 為 "reserved"
	reservation.ActualEndTime = nil
	reservation.Status = "reserved"

	// 插入預約記錄
	if err := database.DB.Create(reservation).Error; err != nil {
		log.Printf("Failed to reserve parking spot: spot_id=%d, error=%v", reservation.SpotID, err)
		return fmt.Errorf("failed to reserve parking spot: %w", err)
	}

	log.Printf("Successfully reserved parking spot: spot_id=%d, reservation_id=%d", reservation.SpotID, reservation.RentID)
	return nil
}

// CheckExpiredReservations 檢查超時的預約記錄
func CheckExpiredReservations() error {
	var reservations []models.Rent
	cstZone := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(cstZone)

	// 查詢所有 status 為 reserved 且 start_time 已過期的記錄
	if err := database.DB.Where("status = ? AND start_time < ?", "reserved", now).Find(&reservations).Error; err != nil {
		log.Printf("Failed to query expired reservations: error=%v", err)
		return fmt.Errorf("failed to query expired reservations: %w", err)
	}

	if len(reservations) == 0 {
		log.Printf("No expired reservations found at %v", now)
		return nil
	}

	// 開始事務
	tx := database.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			log.Printf("Panic occurred during expired reservations check: error=%v", r)
		}
	}()

	for _, reservation := range reservations {
		// 更新狀態為 canceled
		reservation.Status = "canceled"
		if err := tx.Save(&reservation).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to cancel reservation: reservation_id=%d, error=%v", reservation.RentID, err)
			return fmt.Errorf("failed to cancel reservation %d: %w", reservation.RentID, err)
		}

		// 更新車位狀態
		var spot models.ParkingSpot
		if err := tx.Where("spot_id = ?", reservation.SpotID).First(&spot).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to query parking spot: spot_id=%d, error=%v", reservation.SpotID, err)
			return fmt.Errorf("failed to query parking spot for reservation %d: %w", reservation.RentID, err)
		}

		newStatus, err := UpdateParkingSpotStatus(tx, reservation.SpotID, now, cstZone)
		if err != nil {
			tx.Rollback()
			log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", reservation.SpotID, err)
			return fmt.Errorf("failed to update parking spot status for reservation %d: %w", reservation.RentID, err)
		}
		spot.Status = newStatus
		if err := tx.Save(&spot).Error; err != nil {
			tx.Rollback()
			log.Printf("Failed to update parking spot status in DB: spot_id=%d, error=%v", spot.SpotID, err)
			return fmt.Errorf("failed to update parking spot status for reservation %d: %w", reservation.RentID, err)
		}
	}

	// 提交事務
	if err := tx.Commit().Error; err != nil {
		log.Printf("Failed to commit transaction during expired reservations check: error=%v", err)
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("Successfully canceled %d expired reservations at %v", len(reservations), now)
	return nil
}

// UpdateParkingSpotStatus 統一更新車位狀態
func UpdateParkingSpotStatus(tx *gorm.DB, spotID int, now time.Time, cstZone *time.Location) (string, error) {
	var pendingCount int64
	var reservedCount int64
	if err := tx.Model(&models.Rent{}).
		Where("spot_id = ? AND status = ? AND end_time >= ?", spotID, "pending", now).
		Count(&pendingCount).Error; err != nil {
		log.Printf("Failed to check pending rents: spot_id=%d, error=%v", spotID, err)
		return "", fmt.Errorf("failed to check pending rents: %w", err)
	}
	if err := tx.Model(&models.Rent{}).
		Where("spot_id = ? AND status = ? AND end_time >= ?", spotID, "reserved", now).
		Count(&reservedCount).Error; err != nil {
		log.Printf("Failed to check reserved rents: spot_id=%d, error=%v", spotID, err)
		return "", fmt.Errorf("failed to check reserved rents: %w", err)
	}

	var isDayAvailable bool
	todayStr := now.Format("2006-01-02")
	var availableDayCount int64
	if err := tx.Model(&models.ParkingSpotAvailableDay{}).
		Where("parking_spot_id = ? AND available_date = ? AND is_available = ?", spotID, todayStr, true).
		Count(&availableDayCount).Error; err != nil {
		log.Printf("Failed to check available days: spot_id=%d, date=%s, error=%v", spotID, todayStr, err)
		return "", fmt.Errorf("failed to check available days: %w", err)
	}
	isDayAvailable = availableDayCount > 0

	if pendingCount > 0 {
		return "pending", nil
	} else if reservedCount > 0 {
		return "reserved", nil
	} else if isDayAvailable {
		return "available", nil
	}
	return "occupied", nil
}

// GetCurrentlyRentedSpots 查詢目前正在租用中的車位
func GetCurrentlyRentedSpots(memberID int, role string) ([]models.Rent, error) {
	var rents []models.Rent
	cstZone := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(cstZone)

	query := database.DB.
		Preload("Member").
		Preload("ParkingSpot").
		Preload("ParkingSpot.Member").
		Where("status IN (?) AND (actual_end_time IS NULL OR actual_end_time > ?) AND end_time > ?", []string{"pending", "reserved"}, now, now)

	if role == "renter" {
		query = query.Where("member_id = ?", memberID)
	} else if role == "shared_owner" {
		query = query.Joins("JOIN parking_spot ps ON ps.spot_id = rents.spot_id").
			Where("ps.member_id = ?", memberID)
	} else if role == "admin" {
		// admin 可以查詢所有租賃記錄，無需額外條件
	}

	if err := query.Find(&rents).Error; err != nil {
		log.Printf("Failed to query currently rented spots: error=%v", err)
		return nil, fmt.Errorf("failed to query currently rented spots: %w", err)
	}

	// 計算費用並同步車位狀態
	for i := range rents {
		var spot models.ParkingSpot
		if err := database.DB.First(&spot, rents[i].SpotID).Error; err != nil {
			log.Printf("Failed to query parking spot: spot_id=%d, error=%v", rents[i].SpotID, err)
			continue
		}

		// 計算基礎費用（使用當前時間）
		totalCost, err := CalculateRentCost(rents[i], spot, now)
		if err != nil {
			log.Printf("Failed to calculate cost for rent: rent_id=%d, error=%v", rents[i].RentID, err)
			continue
		}
		rents[i].TotalCost = totalCost

		// 同步車位狀態
		if rents[i].Status == "pending" && spot.Status != "pending" {
			spot.Status = "pending"
			if err := database.DB.Save(&spot).Error; err != nil {
				log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", spot.SpotID, err)
			}
		} else if rents[i].Status == "reserved" && spot.Status != "reserved" {
			spot.Status = "reserved"
			if err := database.DB.Save(&spot).Error; err != nil {
				log.Printf("Failed to update parking spot status: spot_id=%d, error=%v", spot.SpotID, err)
			}
		}
	}

	log.Printf("Successfully fetched %d currently rented spots for member_id=%d, role=%s", len(rents), memberID, role)
	return rents, nil
}

// GetAllReservations 查詢所有 reserved 狀態的記錄
func GetAllReservations(memberID int, role string) ([]models.Rent, error) {
	var reservations []models.Rent
	cstZone := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(cstZone)

	query := database.DB.
		Preload("Member").
		Preload("ParkingSpot").
		Preload("ParkingSpot.Member").
		Where("status IN (?, ?)", "reserved", "pending").
		Where("end_time > ?", now)

	switch role {
	case "renter":
		query = query.Where("member_id = ?", memberID)
	case "shared_owner":
		query = query.Joins("JOIN parking_spot ps ON ps.spot_id = rents.spot_id").
			Where("ps.member_id = ?", memberID)
	case "admin":
		// admin 可以查詢所有 reserved 和 pending 記錄，無需額外條件
	default:
		log.Printf("Insufficient role permissions: role=%s", role)
		return nil, fmt.Errorf("insufficient role permissions: role=%s", role)
	}

	if err := query.Find(&reservations).Error; err != nil {
		log.Printf("Failed to query reservations: error=%v", err)
		return nil, fmt.Errorf("failed to fetch reservations: database error: %w", err)
	}

	for i := range reservations {
		reservations[i].StartTime = reservations[i].StartTime.In(cstZone)
		reservations[i].EndTime = reservations[i].EndTime.In(cstZone)
		if reservations[i].ActualEndTime != nil {
			*reservations[i].ActualEndTime = reservations[i].ActualEndTime.In(cstZone)
		}

		if reservations[i].ParkingSpot.SpotID == 0 {
			log.Printf("Parking spot not found for reservation: rent_id=%d, spot_id=%d", reservations[i].RentID, reservations[i].SpotID)
			continue
		}
	}

	log.Printf("Successfully fetched %d reservations for member_id=%d, role=%s", len(reservations), memberID, role)
	return reservations, nil
}
