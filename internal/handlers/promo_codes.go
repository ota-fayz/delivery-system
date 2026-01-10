package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"delivery-system/internal/logger"
	"delivery-system/internal/models"
	"delivery-system/internal/services"

	"github.com/google/uuid"
)

// PromoCodeHandler представляет обработчик промокодов
type PromoCodeHandler struct {
	promoCodeService *services.PromoCodeService
	log              *logger.Logger
}

// NewPromoCodeHandler создает новый обработчик промокодов
func NewPromoCodeHandler(promoCodeService *services.PromoCodeService, log *logger.Logger) *PromoCodeHandler {
	return &PromoCodeHandler{
		promoCodeService: promoCodeService,
		log:              log,
	}
}

// CreatePromoCode создает новый промокод (POST /api/promo-codes)
func (h *PromoCodeHandler) CreatePromoCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req models.CreatePromoCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Валидация запроса
	if err := req.Validate(); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, err.Error())
		return
	}

	promoCode, err := h.promoCodeService.CreatePromoCode(&req)
	if err != nil {
		if err == models.ErrPromoCodeAlreadyExists {
			writeErrorResponse(w, http.StatusConflict, err.Error())
			return
		}
		h.log.WithError(err).Error("Failed to create promo code")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to create promo code")
		return
	}

	writeJSONResponse(w, http.StatusCreated, promoCode)
}

// GetPromoCode получает промокод по ID (GET /api/promo-codes/{id})
func (h *PromoCodeHandler) GetPromoCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлечение ID из пути
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid promo code ID")
		return
	}

	idStr := pathParts[2]
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid promo code ID format")
		return
	}

	promoCode, err := h.promoCodeService.GetPromoCode(id)
	if err != nil {
		if err == models.ErrPromoCodeNotFound {
			writeErrorResponse(w, http.StatusNotFound, "Promo code not found")
			return
		}
		h.log.WithError(err).Error("Failed to get promo code")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get promo code")
		return
	}

	writeJSONResponse(w, http.StatusOK, promoCode)
}

// GetAllPromoCodes получает список всех промокодов (GET /api/promo-codes)
func (h *PromoCodeHandler) GetAllPromoCodes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлечение параметров запроса
	queryParams := r.URL.Query()

	var isActive *bool
	if isActiveStr := queryParams.Get("is_active"); isActiveStr != "" {
		val := isActiveStr == "true"
		isActive = &val
	}

	limit, _ := strconv.Atoi(queryParams.Get("limit"))
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	offset, _ := strconv.Atoi(queryParams.Get("offset"))
	if offset < 0 {
		offset = 0
	}

	promoCodes, err := h.promoCodeService.GetAllPromoCodes(isActive, limit, offset)
	if err != nil {
		h.log.WithError(err).Error("Failed to get promo codes")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to get promo codes")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]interface{}{
		"promo_codes": promoCodes,
		"limit":       limit,
		"offset":      offset,
		"count":       len(promoCodes),
	})
}

// UpdatePromoCode обновляет промокод (PUT /api/promo-codes/{id})
func (h *PromoCodeHandler) UpdatePromoCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлечение ID из пути
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid promo code ID")
		return
	}

	idStr := pathParts[2]
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid promo code ID format")
		return
	}

	var req models.UpdatePromoCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	promoCode, err := h.promoCodeService.UpdatePromoCode(id, &req)
	if err != nil {
		if err == models.ErrPromoCodeNotFound {
			writeErrorResponse(w, http.StatusNotFound, "Promo code not found")
			return
		}
		h.log.WithError(err).Error("Failed to update promo code")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to update promo code")
		return
	}

	writeJSONResponse(w, http.StatusOK, promoCode)
}

// DeletePromoCode удаляет (деактивирует) промокод (DELETE /api/promo-codes/{id})
func (h *PromoCodeHandler) DeletePromoCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	// Извлечение ID из пути
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 3 {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid promo code ID")
		return
	}

	idStr := pathParts[2]
	id, err := uuid.Parse(idStr)
	if err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid promo code ID format")
		return
	}

	if err := h.promoCodeService.DeletePromoCode(id); err != nil {
		if err == models.ErrPromoCodeNotFound {
			writeErrorResponse(w, http.StatusNotFound, "Promo code not found")
			return
		}
		h.log.WithError(err).Error("Failed to delete promo code")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to delete promo code")
		return
	}

	writeJSONResponse(w, http.StatusOK, map[string]string{
		"message": "Promo code deleted successfully",
	})
}

// ValidatePromoCode валидирует промокод (POST /api/promo-codes/validate)
func (h *PromoCodeHandler) ValidatePromoCode(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeErrorResponse(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	var req models.ValidatePromoCodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	if req.Code == "" {
		writeErrorResponse(w, http.StatusBadRequest, "Promo code is required")
		return
	}

	if req.OrderAmount <= 0 {
		writeErrorResponse(w, http.StatusBadRequest, "Order amount must be greater than 0")
		return
	}

	result, err := h.promoCodeService.ValidatePromoCode(&req)
	if err != nil {
		h.log.WithError(err).Error("Failed to validate promo code")
		writeErrorResponse(w, http.StatusInternalServerError, "Failed to validate promo code")
		return
	}

	writeJSONResponse(w, http.StatusOK, result)
}