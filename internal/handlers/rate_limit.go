package handlers

import (
	"encoding/json"
	"net/http"
	"strings"

	"delivery-system/internal/logger"
	"delivery-system/internal/services"
)

// RateLimitHandler обрабатывает запросы связанные с rate limiting
type RateLimitHandler struct {
	rateLimiter *services.RateLimiterService
	log         *logger.Logger
}

// NewRateLimitHandler создает новый RateLimitHandler
func NewRateLimitHandler(rateLimiter *services.RateLimiterService, log *logger.Logger) *RateLimitHandler {
	return &RateLimitHandler{
		rateLimiter: rateLimiter,
		log:         log,
	}
}

// getClientIP извлекает IP адрес клиента из запроса
func getClientIP(r *http.Request) string {
	// Проверяем X-Forwarded-For (если за proxy)
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		return strings.TrimSpace(parts[0])
	}

	// Проверяем X-Real-IP
	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	// Используем RemoteAddr
	ip := r.RemoteAddr
	// Убираем порт (формат "192.168.1.1:54321")
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}

	return ip
}

// GetStatus возвращает текущий статус rate limit для клиента
func (h *RateLimitHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	// Получаем IP адрес клиента
	ip := getClientIP(r)

	// Получаем статус (БЕЗ инкремента счетчика)
	result, err := h.rateLimiter.GetStatus(r.Context(), ip, false)
	if err != nil {
		h.log.Error("Ошибка получения rate limit статуса", "ip", ip, "error", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Формируем JSON ответ
	response := map[string]interface{}{
		"ip":        ip,
		"limit":     result.Limit,
		"remaining": result.Remaining,
		"reset_at":  result.ResetAt.Format("2006-01-02T15:04:05Z07:00"),
		"is_banned": !result.Allowed,
	}

	// Если пользователь забанен, добавляем дополнительную информацию
	if !result.Allowed {
		response["banned_until"] = result.BannedUntil.Format("2006-01-02T15:04:05Z07:00")
		response["retry_after"] = result.RetryAfter
	}

	// Устанавливаем Content-Type и отправляем JSON
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}