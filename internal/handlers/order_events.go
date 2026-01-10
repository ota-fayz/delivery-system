package handlers

import (
	"net/http"
	"strconv"
	"strings"

	"delivery-system/internal/logger"
	"delivery-system/internal/services"
)

// OrderEventsHandler представляет обработчик событий заказов (Event Sourcing)
type OrderEventsHandler struct {
	eventStore *services.EventStore
	log        *logger.Logger
}

// NewOrderEventsHandler создает новый обработчик событий заказов
func NewOrderEventsHandler(eventStore *services.EventStore, log *logger.Logger) *OrderEventsHandler {
	return &OrderEventsHandler{
		eventStore: eventStore,
		log:        log,
	}
}

// GetOrderEvents возвращает все события заказа
// GET /api/orders/{id}/events
func (h *OrderEventsHandler) GetOrderEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлекаем ID заказа из URL
	orderID, err := extractOrderID(r.URL.Path)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid order ID")
		return
	}

	// Получаем события
	events, err := h.eventStore.GetEvents(orderID)
	if err != nil {
		h.log.WithError(err).Error("Failed to get events")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get events")
		return
	}

	h.log.WithField("order_id", orderID).
		WithField("events_count", len(events)).
		Info("Events retrieved successfully")

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"order_id": orderID,
		"count":    len(events),
		"events":   events,
	})
}

// GetOrderEventsAfterVersion возвращает события после указанной версии
// GET /api/orders/{id}/events/{version}
func (h *OrderEventsHandler) GetOrderEventsAfterVersion(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлекаем ID заказа и версию из URL
	orderID, version, err := extractOrderIDAndVersion(r.URL.Path)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid order ID or version")
		return
	}

	// Получаем события после версии
	events, err := h.eventStore.GetEventsAfterVersion(orderID, version)
	if err != nil {
		h.log.WithError(err).Error("Failed to get events after version")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get events")
		return
	}

	h.log.WithField("order_id", orderID).
		WithField("after_version", version).
		WithField("events_count", len(events)).
		Info("Events after version retrieved successfully")

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"order_id":      orderID,
		"after_version": version,
		"count":         len(events),
		"events":        events,
	})
}

// ReplayOrder восстанавливает состояние заказа из событий
// POST /api/orders/{id}/replay
func (h *OrderEventsHandler) ReplayOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлекаем ID заказа из URL
	orderID, err := extractOrderIDFromReplay(r.URL.Path)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid order ID")
		return
	}

	// Восстанавливаем состояние заказа из событий
	order, err := h.eventStore.ReplayOrder(orderID)
	if err != nil {
		h.log.WithError(err).Error("Failed to replay order")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to replay order")
		return
	}

	h.log.WithField("order_id", orderID).
		Info("Order replayed successfully")

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"order_id": orderID,
		"message":  "Order state replayed from events",
		"order":    order,
	})
}

// GetOrderTimeline возвращает временную шкалу событий заказа
// GET /api/orders/{id}/timeline
func (h *OrderEventsHandler) GetOrderTimeline(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлекаем ID заказа из URL
	orderID, err := extractOrderIDFromTimeline(r.URL.Path)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid order ID")
		return
	}

	// Получаем временную шкалу
	timeline, err := h.eventStore.GetTimeline(orderID)
	if err != nil {
		h.log.WithError(err).Error("Failed to get timeline")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get timeline")
		return
	}

	h.log.WithField("order_id", orderID).
		WithField("timeline_count", len(timeline)).
		Info("Timeline retrieved successfully")

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"order_id": orderID,
		"count":    len(timeline),
		"timeline": timeline,
	})
}

// Вспомогательные функции для извлечения параметров из URL

// extractOrderID извлекает ID заказа из URL вида /api/orders/{id}/events
func extractOrderID(path string) (string, error) {
	// Путь: /api/orders/{id}/events
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		return "", ErrInvalidPath
	}

	// parts[3] - это UUID заказа (api/orders/{id}/events)
	orderID := parts[2]
	if orderID == "" {
		return "", ErrInvalidPath
	}

	return orderID, nil
}

// extractOrderIDAndVersion извлекает ID заказа и версию из URL вида /api/orders/{id}/events/{version}
func extractOrderIDAndVersion(path string) (string, int, error) {
	// Путь: /api/orders/{id}/events/{version}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 5 {
		return "", 0, ErrInvalidPath
	}

	// parts[2] - UUID заказа (api/orders/{id}), parts[4] - version
	orderID := parts[2]
	if orderID == "" {
		return "", 0, ErrInvalidPath
	}

	version, err := strconv.Atoi(parts[4])
	if err != nil {
		return "", 0, err
	}

	return orderID, version, nil
}

// extractOrderIDFromReplay извлекает ID заказа из URL вида /api/orders/{id}/replay
func extractOrderIDFromReplay(path string) (string, error) {
	// Путь: /api/orders/{id}/replay
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		return "", ErrInvalidPath
	}

	// parts[2] - это UUID заказа (api/orders/{id})
	orderID := parts[2]
	if orderID == "" {
		return "", ErrInvalidPath
	}

	return orderID, nil
}

// extractOrderIDFromTimeline извлекает ID заказа из URL вида /api/orders/{id}/timeline
func extractOrderIDFromTimeline(path string) (string, error) {
	// Путь: /api/orders/{id}/timeline
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 4 {
		return "", ErrInvalidPath
	}

	// parts[2] - это UUID заказа (api/orders/{id})
	orderID := parts[2]
	if orderID == "" {
		return "", ErrInvalidPath
	}

	return orderID, nil
}
