package models

import (
	"time"

	"github.com/google/uuid"
)

// Review представляет отзыв клиента о курьере после доставки
type Review struct {
	ID        uuid.UUID  `json:"id" db:"id"`
	OrderID   uuid.UUID  `json:"order_id" db:"order_id"`
	CourierID uuid.UUID  `json:"courier_id" db:"courier_id"`
	Rating    int        `json:"rating" db:"rating"`
	Comment   *string    `json:"comment,omitempty" db:"comment"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
}

// CreateReviewRequest представляет запрос на создание отзыва
type CreateReviewRequest struct {
	Rating  int     `json:"rating"`
	Comment *string `json:"comment,omitempty"`
}

