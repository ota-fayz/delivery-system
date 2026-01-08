package models

import (
	"time"

	"github.com/google/uuid"
)

// CourierScore представляет оценку курьера для автоназначения
type CourierScore struct {
	Courier       *Courier `json:"courier"`
	DistanceKm    float64  `json:"distance_km"`    // расстояние до ресторана в км
	DistanceScore float64  `json:"distance_score"` // оценка расстояния (0-1, больше - лучше)
	RatingScore   float64  `json:"rating_score"`   // оценка рейтинга (0-1, больше - лучше)
	LoadScore     float64  `json:"load_score"`     // оценка загруженности (0-1, больше - лучше)
	TotalScore    float64  `json:"total_score"`    // итоговая оценка (0-1, больше - лучше)
	ActiveOrders  int      `json:"active_orders"`  // количество активных заказов
}

// AutoAssignmentResponse представляет ответ на запрос автоназначения курьера
type AutoAssignmentResponse struct {
	Success         bool          `json:"success"`
	OrderID         uuid.UUID     `json:"order_id"`
	AssignedCourier *CourierScore `json:"assigned_courier,omitempty"`
	Message         string        `json:"message"`
	Timestamp       time.Time     `json:"timestamp"`

	// Дополнительная информация для отладки (опционально)
	TotalCandidates int            `json:"total_candidates,omitempty"`
	TopCandidates   []CourierScore `json:"top_candidates,omitempty"` // топ-3 кандидата
}

// AssignmentReason представляет причину выбора курьера
type AssignmentReason string

const (
	AssignmentReasonOptimal       AssignmentReason = "optimal"        // оптимальный выбор по всем критериям
	AssignmentReasonClosest       AssignmentReason = "closest"        // ближайший курьер
	AssignmentReasonBestRating    AssignmentReason = "best_rating"    // лучший рейтинг
	AssignmentReasonLeastBusy     AssignmentReason = "least_busy"     // наименее загружен
	AssignmentReasonOnlyAvailable AssignmentReason = "only_available" // единственный доступный
	AssignmentReasonFallback      AssignmentReason = "fallback"       // запасной вариант (все заняты)
)

// AssignmentLog представляет лог назначения для внутреннего использования
type AssignmentLog struct {
	OrderID         uuid.UUID        `json:"order_id"`
	CourierID       uuid.UUID        `json:"courier_id"`
	Reason          AssignmentReason `json:"reason"`
	Score           float64          `json:"score"`
	DistanceKm      float64          `json:"distance_km"`
	ActiveOrders    int              `json:"active_orders"`
	Timestamp       time.Time        `json:"timestamp"`
	TotalCandidates int              `json:"total_candidates"`
}
