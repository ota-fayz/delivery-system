package models

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// DiscountType представляет тип скидки
type DiscountType string

const (
	DiscountTypeFixedAmount  DiscountType = "fixed_amount"
	DiscountTypePercentage   DiscountType = "percentage"
	DiscountTypeFreeDelivery DiscountType = "free_delivery"
)

// PromoCode представляет промокод в системе
type PromoCode struct {
	ID            uuid.UUID    `json:"id" db:"id"`
	Code          string       `json:"code" db:"code"`
	DiscountType  DiscountType `json:"discount_type" db:"discount_type"`
	DiscountValue float64      `json:"discount_value" db:"discount_value"`
	ValidFrom     time.Time    `json:"valid_from" db:"valid_from"`
	ValidUntil    time.Time    `json:"valid_until" db:"valid_until"`
	UsageLimit    *int         `json:"usage_limit,omitempty" db:"usage_limit"`
	UsedCount     int          `json:"used_count" db:"used_count"`
	IsActive      bool         `json:"is_active" db:"is_active"`
	CreatedAt     time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt     time.Time    `json:"updated_at" db:"updated_at"`
}

// CreatePromoCodeRequest представляет запрос на создание промокода
type CreatePromoCodeRequest struct {
	Code          string       `json:"code"`
	DiscountType  DiscountType `json:"discount_type"`
	DiscountValue float64      `json:"discount_value"`
	ValidFrom     time.Time    `json:"valid_from"`
	ValidUntil    time.Time    `json:"valid_until"`
	UsageLimit    *int         `json:"usage_limit,omitempty"`
}

// UpdatePromoCodeRequest представляет запрос на обновление промокода
type UpdatePromoCodeRequest struct {
	DiscountValue *float64   `json:"discount_value,omitempty"`
	ValidUntil    *time.Time `json:"valid_until,omitempty"`
	UsageLimit    *int       `json:"usage_limit,omitempty"`
	IsActive      *bool      `json:"is_active,omitempty"`
}

// ValidatePromoCodeRequest представляет запрос на валидацию промокода
type ValidatePromoCodeRequest struct {
	Code        string  `json:"code"`
	OrderAmount float64 `json:"order_amount"`
}

// PromoCodeValidationResult представляет результат валидации промокода
type PromoCodeValidationResult struct {
	Valid          bool    `json:"valid"`
	DiscountAmount float64 `json:"discount_amount"`
	FinalAmount    float64 `json:"final_amount"`
	Message        string  `json:"message,omitempty"`
}

var (
	ErrPromoCodeNotFound          = errors.New("промокод не найден")
	ErrPromoCodeInactive          = errors.New("промокод не активен")
	ErrPromoCodeExpired           = errors.New("срок действия промокода истек")
	ErrPromoCodeNotStarted        = errors.New("промокод еще не активирован")
	ErrPromoCodeUsageLimitReached = errors.New("достигнут лимит использования промокода")
	ErrPromoCodeAlreadyExists     = errors.New("промокод с таким кодом уже существует")
	ErrInvalidDiscountType        = errors.New("неверный тип скидки")
	ErrInvalidDiscountValue       = errors.New("неверное значение скидки")
	ErrInvalidDateRange           = errors.New("дата окончания должна быть позже даты начала")
)

// IsValid проверяет, можно ли использовать промокод в данный момент
func (p *PromoCode) IsValid() error {
	if !p.IsActive {
		return ErrPromoCodeInactive
	}

	now := time.Now()
	if now.Before(p.ValidFrom) {
		return ErrPromoCodeNotStarted
	}

	if now.After(p.ValidUntil) {
		return ErrPromoCodeExpired
	}

	if p.UsageLimit != nil && p.UsedCount >= *p.UsageLimit {
		return ErrPromoCodeUsageLimitReached
	}

	return nil
}

// CalculateDiscount рассчитывает сумму скидки для заданной суммы заказа
func (p *PromoCode) CalculateDiscount(orderAmount float64, deliveryFee float64) float64 {
	switch p.DiscountType {
	case DiscountTypeFixedAmount:
		if p.DiscountValue > orderAmount {
			return orderAmount
		}
		return p.DiscountValue

	case DiscountTypePercentage:
		discount := orderAmount * (p.DiscountValue / 100.0)
		if discount > orderAmount {
			return orderAmount
		}
		return discount

	case DiscountTypeFreeDelivery:
		return deliveryFee

	default:
		return 0
	}
}

// Validate валидирует запрос на создание промокода
func (req *CreatePromoCodeRequest) Validate() error {
	if req.Code == "" {
		return errors.New("код промокода обязателен")
	}

	if len(req.Code) > 50 {
		return errors.New("код промокода не может быть длиннее 50 символов")
	}

	if req.DiscountType != DiscountTypeFixedAmount &&
		req.DiscountType != DiscountTypePercentage &&
		req.DiscountType != DiscountTypeFreeDelivery {
		return ErrInvalidDiscountType
	}

	if req.DiscountValue < 0 {
		return ErrInvalidDiscountValue
	}

	if req.DiscountType == DiscountTypePercentage && req.DiscountValue > 100 {
		return errors.New("процент скидки не может быть больше 100")
	}

	if req.ValidUntil.Before(req.ValidFrom) || req.ValidUntil.Equal(req.ValidFrom) {
		return ErrInvalidDateRange
	}

	if req.UsageLimit != nil && *req.UsageLimit <= 0 {
		return errors.New("лимит использований должен быть больше 0")
	}

	return nil
}