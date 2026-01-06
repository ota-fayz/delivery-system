package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"delivery-system/internal/database"
	"delivery-system/internal/logger"
	"delivery-system/internal/models"

	"github.com/google/uuid"
)

var (
	ErrReviewAlreadyExists = errors.New("review for this order already exists")
	ErrOrderNotDelivered   = errors.New("order must be delivered to leave a review")
	ErrOrderHasNoCourier   = errors.New("order has no assigned courier")
	ErrInvalidRating       = errors.New("rating must be between 1 and 5")
)

// ReviewService предоставляет бизнес-логику для работы с отзывами
type ReviewService struct {
	db     *database.DB
	logger *logger.Logger
}

// NewReviewService создаёт новый экземпляр сервиса отзывов
func NewReviewService(db *database.DB, logger *logger.Logger) *ReviewService {
	return &ReviewService{
		db:     db,
		logger: logger,
	}
}

// CreateReview создаёт новый отзыв и обновляет рейтинг курьера
func (s *ReviewService) CreateReview(ctx context.Context, orderID uuid.UUID, req models.CreateReviewRequest) (*models.Review, error) {
	// 1. Валидация рейтинга
	if req.Rating < 1 || req.Rating > 5 {
		return nil, ErrInvalidRating
	}

	// 2. Начинаем транзакцию (чтобы все изменения были атомарными)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("Failed to begin transaction", "error", err)
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback() // Откатим, если что-то пойдёт не так

	// 3. Получаем информацию о заказе
	var order models.Order
	err = tx.QueryRowContext(ctx, `
		SELECT id, status, courier_id
		FROM orders
		WHERE id = $1
	`, orderID).Scan(&order.ID, &order.Status, &order.CourierID)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("order not found")
	}
	if err != nil {
		s.logger.Error("Failed to get order", "error", err)
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// 4. Проверяем, что заказ доставлен
	if order.Status != models.OrderStatusDelivered {
		return nil, ErrOrderNotDelivered
	}

	// 5. Проверяем, что у заказа есть курьер
	if order.CourierID == nil {
		return nil, ErrOrderHasNoCourier
	}

	// 6. Создаём отзыв
	review := &models.Review{
		ID:        uuid.New(),
		OrderID:   orderID,
		CourierID: *order.CourierID,
		Rating:    req.Rating,
		Comment:   req.Comment,
	}

	err = tx.QueryRowContext(ctx, `
		INSERT INTO reviews (id, order_id, courier_id, rating, comment)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING created_at
	`, review.ID, review.OrderID, review.CourierID, review.Rating, review.Comment).Scan(&review.CreatedAt)

	if err != nil {
		s.logger.Error("Failed to create review", "error", err)
		// Проверяем, не нарушили ли UNIQUE constraint
		if err.Error() == `pq: duplicate key value violates unique constraint "reviews_order_id_key"` {
			return nil, ErrReviewAlreadyExists
		}
		return nil, fmt.Errorf("failed to create review: %w", err)
	}

	// 7. Обновляем рейтинг курьера
	err = s.updateCourierRating(ctx, tx, *order.CourierID, req.Rating)
	if err != nil {
		return nil, err
	}

	// 8. Коммитим транзакцию
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit transaction", "error", err)
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	s.logger.Info("Review created",
		"review_id", review.ID,
		"order_id", orderID,
		"courier_id", review.CourierID,
		"rating", review.Rating)

	return review, nil
}

// updateCourierRating обновляет средний рейтинг курьера
func (s *ReviewService) updateCourierRating(ctx context.Context, tx *sql.Tx, courierID uuid.UUID, newRating int) error {
	// Получаем текущий рейтинг и количество отзывов
	var currentRating sql.NullFloat64
	var totalReviews int

	err := tx.QueryRowContext(ctx, `
		SELECT rating, total_reviews
		FROM couriers
		WHERE id = $1
	`, courierID).Scan(&currentRating, &totalReviews)

	if err != nil {
		s.logger.Error("Failed to get courier rating", "error", err)
		return fmt.Errorf("failed to get courier rating: %w", err)
	}

	// Вычисляем новый средний рейтинг
	var newAvgRating float64
	if currentRating.Valid {
		// У курьера уже есть рейтинг
		newAvgRating = (currentRating.Float64*float64(totalReviews) + float64(newRating)) / float64(totalReviews+1)
	} else {
		// Это первый отзыв
		newAvgRating = float64(newRating)
	}

	// Округляем до 2 знаков после запятой
	newAvgRating = math.Round(newAvgRating*100) / 100

	// Обновляем рейтинг и счётчик
	_, err = tx.ExecContext(ctx, `
		UPDATE couriers
		SET rating = $1, total_reviews = total_reviews + 1
		WHERE id = $2
	`, newAvgRating, courierID)

	if err != nil {
		s.logger.Error("Failed to update courier rating", "error", err)
		return fmt.Errorf("failed to update courier rating: %w", err)
	}

	s.logger.Info("Courier rating updated",
		"courier_id", courierID,
		"new_rating", newAvgRating,
		"total_reviews", totalReviews+1)

	return nil
}

// GetCourierReviews получает все отзывы для курьера
func (s *ReviewService) GetCourierReviews(ctx context.Context, courierID uuid.UUID) ([]models.Review, error) {
	query := `
		SELECT id, order_id, courier_id, rating, comment, created_at
		FROM reviews
		WHERE courier_id = $1
		ORDER BY created_at DESC
	`

	rows, err := s.db.QueryContext(ctx, query, courierID)
	if err != nil {
		s.logger.Error("Failed to get courier reviews", "error", err, "courier_id", courierID)
		return nil, fmt.Errorf("failed to get courier reviews: %w", err)
	}
	defer rows.Close()

	var reviews []models.Review
	for rows.Next() {
		var review models.Review
		err := rows.Scan(
			&review.ID,
			&review.OrderID,
			&review.CourierID,
			&review.Rating,
			&review.Comment,
			&review.CreatedAt,
		)
		if err != nil {
			s.logger.Error("Failed to scan review", "error", err)
			return nil, fmt.Errorf("failed to scan review: %w", err)
		}
		reviews = append(reviews, review)
	}

	if err := rows.Err(); err != nil {
		s.logger.Error("Error iterating reviews", "error", err)
		return nil, fmt.Errorf("error iterating reviews: %w", err)
	}

	s.logger.Info("Retrieved courier reviews", "courier_id", courierID, "count", len(reviews))

	return reviews, nil
}
