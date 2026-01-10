package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"delivery-system/internal/database"
	"delivery-system/internal/logger"
	"delivery-system/internal/models"

	"github.com/google/uuid"
)

// PromoCodeService представляет сервис для работы с промокодами
type PromoCodeService struct {
	db  *database.DB
	log *logger.Logger
}

// NewPromoCodeService создает новый экземпляр сервиса промокодов
func NewPromoCodeService(db *database.DB, log *logger.Logger) *PromoCodeService {
	return &PromoCodeService{
		db:  db,
		log: log,
	}
}

// CreatePromoCode создает новый промокод
func (s *PromoCodeService) CreatePromoCode(req *models.CreatePromoCodeRequest) (*models.PromoCode, error) {
	// Валидация запроса
	if err := req.Validate(); err != nil {
		return nil, fmt.Errorf("validation failed: %w", err)
	}

	// Нормализация кода (приведение к верхнему регистру)
	code := strings.ToUpper(strings.TrimSpace(req.Code))

	// Проверка на существование промокода с таким кодом
	exists, err := s.promoCodeExists(code)
	if err != nil {
		return nil, fmt.Errorf("failed to check promo code existence: %w", err)
	}
	if exists {
		return nil, models.ErrPromoCodeAlreadyExists
	}

	promoCode := &models.PromoCode{
		ID:            uuid.New(),
		Code:          code,
		DiscountType:  req.DiscountType,
		DiscountValue: req.DiscountValue,
		ValidFrom:     req.ValidFrom,
		ValidUntil:    req.ValidUntil,
		UsageLimit:    req.UsageLimit,
		UsedCount:     0,
		IsActive:      true,
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
	}

	query := `
		INSERT INTO promo_codes (id, code, discount_type, discount_value, valid_from, valid_until, usage_limit, used_count, is_active, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
	`

	_, err = s.db.Exec(query,
		promoCode.ID,
		promoCode.Code,
		promoCode.DiscountType,
		promoCode.DiscountValue,
		promoCode.ValidFrom,
		promoCode.ValidUntil,
		promoCode.UsageLimit,
		promoCode.UsedCount,
		promoCode.IsActive,
		promoCode.CreatedAt,
		promoCode.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create promo code: %w", err)
	}

	s.log.WithFields(map[string]interface{}{
		"promo_code_id": promoCode.ID,
		"code":          promoCode.Code,
		"discount_type": promoCode.DiscountType,
	}).Info("Promo code created successfully")

	return promoCode, nil
}

// GetPromoCode получает промокод по ID
func (s *PromoCodeService) GetPromoCode(id uuid.UUID) (*models.PromoCode, error) {
	promoCode := &models.PromoCode{}

	query := `
		SELECT id, code, discount_type, discount_value, valid_from, valid_until,
		       usage_limit, used_count, is_active, created_at, updated_at
		FROM promo_codes
		WHERE id = $1
	`

	err := s.db.QueryRow(query, id).Scan(
		&promoCode.ID,
		&promoCode.Code,
		&promoCode.DiscountType,
		&promoCode.DiscountValue,
		&promoCode.ValidFrom,
		&promoCode.ValidUntil,
		&promoCode.UsageLimit,
		&promoCode.UsedCount,
		&promoCode.IsActive,
		&promoCode.CreatedAt,
		&promoCode.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrPromoCodeNotFound
		}
		return nil, fmt.Errorf("failed to get promo code: %w", err)
	}

	return promoCode, nil
}

// GetPromoCodeByCode получает промокод по его коду
func (s *PromoCodeService) GetPromoCodeByCode(code string) (*models.PromoCode, error) {
	promoCode := &models.PromoCode{}

	// Нормализация кода
	code = strings.ToUpper(strings.TrimSpace(code))

	query := `
		SELECT id, code, discount_type, discount_value, valid_from, valid_until,
		       usage_limit, used_count, is_active, created_at, updated_at
		FROM promo_codes
		WHERE code = $1
	`

	err := s.db.QueryRow(query, code).Scan(
		&promoCode.ID,
		&promoCode.Code,
		&promoCode.DiscountType,
		&promoCode.DiscountValue,
		&promoCode.ValidFrom,
		&promoCode.ValidUntil,
		&promoCode.UsageLimit,
		&promoCode.UsedCount,
		&promoCode.IsActive,
		&promoCode.CreatedAt,
		&promoCode.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, models.ErrPromoCodeNotFound
		}
		return nil, fmt.Errorf("failed to get promo code by code: %w", err)
	}

	return promoCode, nil
}

// GetAllPromoCodes получает список всех промокодов с фильтрацией
func (s *PromoCodeService) GetAllPromoCodes(isActive *bool, limit, offset int) ([]*models.PromoCode, error) {
	query := `
		SELECT id, code, discount_type, discount_value, valid_from, valid_until,
		       usage_limit, used_count, is_active, created_at, updated_at
		FROM promo_codes
		WHERE 1=1
	`

	args := []interface{}{}
	argIndex := 1

	if isActive != nil {
		query += fmt.Sprintf(" AND is_active = $%d", argIndex)
		args = append(args, *isActive)
		argIndex++
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT $%d", argIndex)
		args = append(args, limit)
		argIndex++
	}

	if offset > 0 {
		query += fmt.Sprintf(" OFFSET $%d", argIndex)
		args = append(args, offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to get promo codes: %w", err)
	}
	defer rows.Close()

	var promoCodes []*models.PromoCode
	for rows.Next() {
		promoCode := &models.PromoCode{}
		if err := rows.Scan(
			&promoCode.ID,
			&promoCode.Code,
			&promoCode.DiscountType,
			&promoCode.DiscountValue,
			&promoCode.ValidFrom,
			&promoCode.ValidUntil,
			&promoCode.UsageLimit,
			&promoCode.UsedCount,
			&promoCode.IsActive,
			&promoCode.CreatedAt,
			&promoCode.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan promo code: %w", err)
		}
		promoCodes = append(promoCodes, promoCode)
	}

	return promoCodes, nil
}

// UpdatePromoCode обновляет промокод
func (s *PromoCodeService) UpdatePromoCode(id uuid.UUID, req *models.UpdatePromoCodeRequest) (*models.PromoCode, error) {
	// Проверка существования промокода
	_, err := s.GetPromoCode(id)
	if err != nil {
		return nil, err
	}

	query := "UPDATE promo_codes SET updated_at = $1"
	args := []interface{}{time.Now()}
	argIndex := 2

	if req.DiscountValue != nil {
		if *req.DiscountValue < 0 {
			return nil, models.ErrInvalidDiscountValue
		}
		query += fmt.Sprintf(", discount_value = $%d", argIndex)
		args = append(args, *req.DiscountValue)
		argIndex++
	}

	if req.ValidUntil != nil {
		query += fmt.Sprintf(", valid_until = $%d", argIndex)
		args = append(args, *req.ValidUntil)
		argIndex++
	}

	if req.UsageLimit != nil {
		if *req.UsageLimit < 0 {
			return nil, fmt.Errorf("usage limit cannot be negative")
		}
		query += fmt.Sprintf(", usage_limit = $%d", argIndex)
		args = append(args, *req.UsageLimit)
		argIndex++
	}

	if req.IsActive != nil {
		query += fmt.Sprintf(", is_active = $%d", argIndex)
		args = append(args, *req.IsActive)
		argIndex++
	}

	query += fmt.Sprintf(" WHERE id = $%d", argIndex)
	args = append(args, id)

	_, err = s.db.Exec(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update promo code: %w", err)
	}

	s.log.WithFields(map[string]interface{}{
		"promo_code_id": id,
	}).Info("Promo code updated successfully")

	return s.GetPromoCode(id)
}

// DeletePromoCode удаляет промокод (мягкое удаление - деактивация)
func (s *PromoCodeService) DeletePromoCode(id uuid.UUID) error {
	query := `
		UPDATE promo_codes
		SET is_active = false, updated_at = $1
		WHERE id = $2
	`

	result, err := s.db.Exec(query, time.Now(), id)
	if err != nil {
		return fmt.Errorf("failed to delete promo code: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return models.ErrPromoCodeNotFound
	}

	s.log.WithFields(map[string]interface{}{
		"promo_code_id": id,
	}).Info("Promo code deleted (deactivated)")

	return nil
}

// ValidatePromoCode валидирует промокод для использования
func (s *PromoCodeService) ValidatePromoCode(req *models.ValidatePromoCodeRequest) (*models.PromoCodeValidationResult, error) {
	// Получение промокода по коду
	promoCode, err := s.GetPromoCodeByCode(req.Code)
	if err != nil {
		return &models.PromoCodeValidationResult{
			Valid:   false,
			Message: "Промокод не найден",
		}, nil
	}

	// Проверка валидности промокода
	if err := promoCode.IsValid(); err != nil {
		return &models.PromoCodeValidationResult{
			Valid:   false,
			Message: err.Error(),
		}, nil
	}

	// Расчет скидки (предполагаем, что доставка стоит 0 для простоты, можно передать как параметр)
	const deliveryFee = 0.0
	discountAmount := promoCode.CalculateDiscount(req.OrderAmount, deliveryFee)
	finalAmount := req.OrderAmount - discountAmount

	// Финальная сумма не может быть отрицательной
	if finalAmount < 0 {
		finalAmount = 0
	}

	return &models.PromoCodeValidationResult{
		Valid:          true,
		DiscountAmount: discountAmount,
		FinalAmount:    finalAmount,
		Message:        "Промокод успешно применен",
	}, nil
}

// ApplyPromoCode применяет промокод и увеличивает счетчик использований
func (s *PromoCodeService) ApplyPromoCode(code string) error {
	// Нормализация кода
	code = strings.ToUpper(strings.TrimSpace(code))

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Получение промокода с блокировкой строки
	query := `
		SELECT id, code, discount_type, discount_value, valid_from, valid_until,
		       usage_limit, used_count, is_active, created_at, updated_at
		FROM promo_codes
		WHERE code = $1
		FOR UPDATE
	`

	promoCode := &models.PromoCode{}
	err = tx.QueryRow(query, code).Scan(
		&promoCode.ID,
		&promoCode.Code,
		&promoCode.DiscountType,
		&promoCode.DiscountValue,
		&promoCode.ValidFrom,
		&promoCode.ValidUntil,
		&promoCode.UsageLimit,
		&promoCode.UsedCount,
		&promoCode.IsActive,
		&promoCode.CreatedAt,
		&promoCode.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return models.ErrPromoCodeNotFound
		}
		return fmt.Errorf("failed to get promo code: %w", err)
	}

	// Проверка валидности
	if err := promoCode.IsValid(); err != nil {
		return err
	}

	// Увеличение счетчика использований
	updateQuery := `
		UPDATE promo_codes
		SET used_count = used_count + 1, updated_at = $1
		WHERE id = $2
	`

	_, err = tx.Exec(updateQuery, time.Now(), promoCode.ID)
	if err != nil {
		return fmt.Errorf("failed to update promo code usage count: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.log.WithFields(map[string]interface{}{
		"promo_code_id": promoCode.ID,
		"code":          promoCode.Code,
		"used_count":    promoCode.UsedCount + 1,
	}).Info("Promo code applied successfully")

	return nil
}

// promoCodeExists проверяет существование промокода по коду
func (s *PromoCodeService) promoCodeExists(code string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS(SELECT 1 FROM promo_codes WHERE code = $1)`
	err := s.db.QueryRow(query, code).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check promo code existence: %w", err)
	}
	return exists, nil
}