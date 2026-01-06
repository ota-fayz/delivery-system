package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"delivery-system/internal/logger"
	"delivery-system/internal/models"
	"delivery-system/internal/services"

	"github.com/google/uuid"
)

// ReviewHandler обрабатывает запросы, связанные с отзывами
type ReviewHandler struct {
	reviewService *services.ReviewService
	log           *logger.Logger
}

// NewReviewHandler создаёт новый обработчик отзывов
func NewReviewHandler(reviewService *services.ReviewService, log *logger.Logger) *ReviewHandler {
	return &ReviewHandler{
		reviewService: reviewService,
		log:           log,
	}
}

// CreateReview создаёт отзыв для заказа
// POST /api/orders/{id}/review
func (h *ReviewHandler) CreateReview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлекаем order_id из URL
	// URL: /api/orders/{id}/review
	path := strings.TrimPrefix(r.URL.Path, "/api/orders/")
	path = strings.TrimSuffix(path, "/review")

	orderID, err := uuid.Parse(path)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid order ID")
		return
	}

	// Парсим тело запроса
	var req models.CreateReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Валидация
	if req.Rating < 1 || req.Rating > 5 {
		writeErrorResponse(w, http.StatusBadRequest, "Rating must be between 1 and 5")
		return
	}

	// Проверка длины комментария
	if req.Comment != nil && len(*req.Comment) > 1000 {
		writeErrorResponse(w, http.StatusBadRequest, "Comment must not exceed 1000 characters")
		return
	}

	// Создаём отзыв
	review, err := h.reviewService.CreateReview(r.Context(), orderID, req)
	if err != nil {
		h.log.Error("Failed to create review", "error", err, "order_id", orderID)

		// Обрабатываем специфичные ошибки
		switch err {
		case services.ErrReviewAlreadyExists:
			writeErrorResponse(w, http.StatusConflict, "Review for this order already exists")
		case services.ErrOrderNotDelivered:
			writeErrorResponse(w, http.StatusBadRequest, "Order must be delivered to leave a review")
		case services.ErrOrderHasNoCourier:
			writeErrorResponse(w, http.StatusBadRequest, "Order has no assigned courier")
		case services.ErrInvalidRating:
			writeErrorResponse(w, http.StatusBadRequest, "Invalid rating value")
		default:
			if strings.Contains(err.Error(), "order not found") {
				writeErrorResponse(w, http.StatusNotFound, "Order not found")
			} else {
				writeErrorResponse(w, http.StatusInternalServerError, "Failed to create review")
			}
		}
		return
	}

	h.log.Info("Review created successfully",
		"review_id", review.ID,
		"order_id", orderID,
		"rating", review.Rating)

	writeJSONResponse(w, http.StatusCreated, review)
}

// GetCourierReviews получает все отзывы курьера
// GET /api/couriers/{id}/reviews
func (h *ReviewHandler) GetCourierReviews(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлекаем courier_id из URL
	// URL: /api/couriers/{id}/reviews
	path := strings.TrimPrefix(r.URL.Path, "/api/couriers/")
	path = strings.TrimSuffix(path, "/reviews")

	courierID, err := uuid.Parse(path)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid courier ID")
		return
	}

	// Получаем отзывы
	reviews, err := h.reviewService.GetCourierReviews(r.Context(), courierID)
	if err != nil {
		h.log.Error("Failed to get courier reviews", "error", err, "courier_id", courierID)
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get reviews")
		return
	}

	h.log.Info("Retrieved courier reviews", "courier_id", courierID, "count", len(reviews))

	// Возвращаем пустой массив вместо null, если отзывов нет
	if reviews == nil {
		reviews = []models.Review{}
	}

	writeJSONResponse(w, http.StatusOK, reviews)
}
