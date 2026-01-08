package handlers

import (
	"encoding/json"
	"net/http"

	"delivery-system/internal/logger"
	"delivery-system/internal/services"

	"github.com/google/uuid"
)

// AssignmentHandler представляет обработчик автоназначения курьеров
type AssignmentHandler struct {
	assignmentService *services.AssignmentService
	log               *logger.Logger
}

// NewAssignmentHandler создает новый обработчик автоназначения
func NewAssignmentHandler(assignmentService *services.AssignmentService, log *logger.Logger) *AssignmentHandler {
	return &AssignmentHandler{
		assignmentService: assignmentService,
		log:               log,
	}
}

// AutoAssignCourier автоматически назначает курьера на заказ
// POST /api/orders/{id}/auto-assign
func (h *AssignmentHandler) AutoAssignCourier(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлекаем order_id из URL
	orderID, err := h.extractOrderIDFromPath(r.URL.Path)
	if err != nil {
		h.log.Warn("Invalid order ID in URL", "path", r.URL.Path, "error", err)
		writeErrorResponse(w, http.StatusBadRequest, "Invalid order ID")
		return
	}

	// Вызываем сервис автоназначения
	ctx := r.Context()
	response, err := h.assignmentService.AutoAssignCourier(ctx, orderID)

	if err != nil {
		h.log.WithError(err).Error("Failed to auto-assign courier", "order_id", orderID)

		// Обрабатываем специфичные ошибки
		switch err {
		case services.ErrOrderNotFound:
			writeErrorResponse(w, http.StatusNotFound, "Order not found")
		case services.ErrOrderAlreadyAssigned:
			writeErrorResponse(w, http.StatusConflict, "Order already assigned to a courier")
		case services.ErrInvalidOrderStatus:
			writeErrorResponse(w, http.StatusBadRequest, "Order status does not allow courier assignment")
		case services.ErrNoAvailableCouriers:
			writeErrorResponse(w, http.StatusServiceUnavailable, "No available couriers at the moment")
		default:
			writeErrorResponse(w, http.StatusInternalServerError, "Failed to assign courier")
		}
		return
	}

	// Возвращаем успешный ответ
	w.Header().Set("Content-Type", "application/json")

	if response.Success {
		w.WriteHeader(http.StatusOK)
		h.log.Info("Courier auto-assigned successfully",
			"order_id", orderID,
			"courier_id", response.AssignedCourier.Courier.ID,
			"score", response.AssignedCourier.TotalScore,
		)
	} else {
		// Успешный ответ, но курьер не найден
		w.WriteHeader(http.StatusOK)
		h.log.Warn("No couriers available for assignment", "order_id", orderID)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		h.log.WithError(err).Error("Failed to encode response")
	}
}

// extractOrderIDFromPath извлекает order_id из URL пути
// Ожидается путь вида: /api/orders/{order_id}/auto-assign
func (h *AssignmentHandler) extractOrderIDFromPath(path string) (uuid.UUID, error) {
	// Простой парсинг пути (можно улучшить с использованием роутера)
	// Пример: /api/orders/123e4567-e89b-12d3-a456-426614174000/auto-assign

	// Убираем префикс и суффикс
	// Путь должен быть: /api/orders/{id}/auto-assign
	const (
		prefix = "/api/orders/"
		suffix = "/auto-assign"
	)

	if len(path) < len(prefix)+len(suffix) {
		return uuid.Nil, ErrInvalidPath
	}

	// Удаляем префикс
	if path[:len(prefix)] != prefix {
		return uuid.Nil, ErrInvalidPath
	}
	path = path[len(prefix):]

	// Находим суффикс
	suffixIndex := len(path) - len(suffix)
	if suffixIndex < 0 || path[suffixIndex:] != suffix {
		return uuid.Nil, ErrInvalidPath
	}

	// Извлекаем ID
	idStr := path[:suffixIndex]
	orderID, err := uuid.Parse(idStr)
	if err != nil {
		return uuid.Nil, err
	}

	return orderID, nil
}
