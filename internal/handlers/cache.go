package handlers

import (
	"net/http"

	"delivery-system/internal/logger"
	"delivery-system/internal/services"
)

// CacheHandler представляет обработчик для кеша
type CacheHandler struct {
	cacheService *services.CacheService
	log          *logger.Logger
}

// NewCacheHandler создает новый обработчик кеша
func NewCacheHandler(cacheService *services.CacheService, log *logger.Logger) *CacheHandler {
	return &CacheHandler{
		cacheService: cacheService,
		log:          log,
	}
}

// GetMetrics возвращает метрики кеширования
func (h *CacheHandler) GetMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	metrics, err := h.cacheService.GetMetrics(r.Context())
	if err != nil {
		h.log.WithError(err).Error("Failed to get cache metrics")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get cache metrics")
		return
	}

	writeJSONResponse(w, http.StatusOK, metrics)
}
