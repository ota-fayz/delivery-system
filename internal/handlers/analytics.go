package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"delivery-system/internal/logger"
	"delivery-system/internal/models"
	"delivery-system/internal/services"
)

// AnalyticsHandler представляет обработчик аналитики
type AnalyticsHandler struct {
	analyticsService *services.AnalyticsService
	log              *logger.Logger
}

// NewAnalyticsHandler создает новый обработчик аналитики
func NewAnalyticsHandler(analyticsService *services.AnalyticsService, log *logger.Logger) *AnalyticsHandler {
	return &AnalyticsHandler{
		analyticsService: analyticsService,
		log:              log,
	}
}

// GetOverview получает общую сводку (GET /api/analytics/overview)
func (h *AnalyticsHandler) GetOverview(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	ctx := context.Background()
	metrics, err := h.analyticsService.GetOverviewMetrics(ctx)
	if err != nil {
		h.log.WithError(err).Error("Failed to get overview metrics")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get overview metrics")
		return
	}

	writeJSONResponse(w, http.StatusOK, metrics)
}

// GetRevenue получает метрики по выручке (GET /api/analytics/revenue)
func (h *AnalyticsHandler) GetRevenue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	metrics, err := h.analyticsService.GetRevenueMetrics(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to get revenue metrics")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get revenue metrics")
		return
	}

	writeJSONResponse(w, http.StatusOK, metrics)
}

// GetOrders получает метрики по заказам (GET /api/analytics/orders)
func (h *AnalyticsHandler) GetOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	metrics, err := h.analyticsService.GetOrderMetrics(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to get order metrics")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get order metrics")
		return
	}

	writeJSONResponse(w, http.StatusOK, metrics)
}

// GetTopCouriers получает топ курьеров (GET /api/analytics/couriers/top)
func (h *AnalyticsHandler) GetTopCouriers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	couriers, err := h.analyticsService.GetTopCouriers(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to get top couriers")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get top couriers")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"couriers": couriers,
		"count":    len(couriers),
		"period":   req.Period,
	})
}

// GetPopularItems получает популярные товары (GET /api/analytics/items/popular)
func (h *AnalyticsHandler) GetPopularItems(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	items, err := h.analyticsService.GetPopularItems(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to get popular items")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get popular items")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"items":  items,
		"count":  len(items),
		"period": req.Period,
	})
}

// GetPromoCodeStats получает статистику по промокодам (GET /api/analytics/promo-codes)
func (h *AnalyticsHandler) GetPromoCodeStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	stats, err := h.analyticsService.GetPromoCodeStats(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to get promo code stats")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get promo code stats")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"promo_codes": stats,
		"count":       len(stats),
		"period":      req.Period,
	})
}

// GetTimeSeries получает временной ряд (GET /api/analytics/timeseries)
func (h *AnalyticsHandler) GetTimeSeries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	data, err := h.analyticsService.GetTimeSeries(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to get time series")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get time series")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"data":   data,
		"count":  len(data),
		"period": req.Period,
	})
}

// GetFullAnalytics получает полную аналитику (GET /api/analytics)
func (h *AnalyticsHandler) GetFullAnalytics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	analytics, err := h.analyticsService.GetAnalytics(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to get full analytics")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get full analytics")
		return
	}

	writeJSONResponse(w, http.StatusOK, analytics)
}

// ExportCSV экспортирует аналитику в CSV (GET /api/analytics/export/csv)
func (h *AnalyticsHandler) ExportCSV(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	req := h.parseAnalyticsRequest(r)
	ctx := context.Background()

	csv, err := h.analyticsService.ExportToCSV(ctx, req)
	if err != nil {
		h.log.WithError(err).Error("Failed to export analytics to CSV")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to export analytics")
		return
	}

	// Установка заголовков для скачивания файла
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=analytics_"+time.Now().Format("2006-01-02")+".csv")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(csv))
}

// parseAnalyticsRequest парсит параметры запроса для аналитики
func (h *AnalyticsHandler) parseAnalyticsRequest(r *http.Request) *models.AnalyticsRequest {
	query := r.URL.Query()

	// Period (по умолчанию - день)
	period := models.AnalyticsPeriod(query.Get("period"))
	if period == "" {
		period = models.PeriodDay
	}

	// Limit (по умолчанию - 10)
	limit := 10
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	// Start Date
	var startDate *time.Time
	if startStr := query.Get("start_date"); startStr != "" {
		if t, err := time.Parse("2006-01-02", startStr); err == nil {
			startDate = &t
		}
	}

	// End Date
	var endDate *time.Time
	if endStr := query.Get("end_date"); endStr != "" {
		if t, err := time.Parse("2006-01-02", endStr); err == nil {
			endDate = &t
		}
	}

	return &models.AnalyticsRequest{
		Period:    period,
		StartDate: startDate,
		EndDate:   endDate,
		Limit:     limit,
	}
}
