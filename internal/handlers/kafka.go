package handlers

import (
	"encoding/json"
	"net/http"

	"delivery-system/internal/kafka"
	"delivery-system/internal/logger"
)

// KafkaHandler обрабатывает запросы связанные с Kafka метриками
type KafkaHandler struct {
	metrics *kafka.EventMetrics
	log     *logger.Logger
}

// NewKafkaHandler создает новый handler для Kafka метрик
func NewKafkaHandler(metrics *kafka.EventMetrics, log *logger.Logger) *KafkaHandler {
	return &KafkaHandler{
		metrics: metrics,
		log:     log,
	}
}

// GetStats возвращает статистику обработки событий Kafka
// GET /api/kafka/stats
func (h *KafkaHandler) GetStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Получаем метрики
	stats := h.metrics.GetStats()

	// Отправляем ответ
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(stats); err != nil {
		h.log.WithError(err).Error("Failed to encode Kafka stats response")
		writeErrorResponse(w, http.StatusInternalServerError, "Internal server error")
		return
	}

	h.log.Debug("Kafka stats retrieved successfully")
}
