package middleware

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"delivery-system/internal/logger"
	"delivery-system/internal/services"
)

func getClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ", ")
		return strings.TrimSpace(parts[0])
	}

	if realIP := r.Header.Get("X-Real-IP"); realIP != "" {
		return realIP
	}

	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}

	return ip
}

func RateLimitMiddleware(rateLimiter *services.RateLimiterService, log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ip := getClientIP(r)
			isVIP := false

			result, err := rateLimiter.CheckLimit(r.Context(), ip, isVIP)
			if err != nil {
				log.Error("Ошибка проверки rate limit", "ip", ip, "error", err)
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("X-RateLimit-Limit", fmt.Sprintf("%d", result.Limit))
			w.Header().Set("X-RateLimit-Remaining", fmt.Sprintf("%d", result.Remaining))
			w.Header().Set("X-RateLimit-Reset", result.ResetAt.Format("2006-01-02T15:04:05Z07:00"))

			if !result.Allowed {
				w.Header().Set("Retry-After", fmt.Sprintf("%d", result.RetryAfter))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)

				response := map[string]interface{}{
					"error":       "rate_limit_exceeded",
					"message":     "Превышен лимит запросов. Повторите попытку позже.",
					"limit":       result.Limit,
					"retry_after": result.RetryAfter,
				}

				if !result.BannedUntil.IsZero() {
					response["banned_until"] = result.BannedUntil.Format("2006-01-02T15:04:05Z07:00")
				}

				log.Warn("Запрос заблокирован rate limiter",
					"ip", ip,
					"path", r.URL.Path,
					"retry_after", result.RetryAfter)

				json.NewEncoder(w).Encode(response)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
