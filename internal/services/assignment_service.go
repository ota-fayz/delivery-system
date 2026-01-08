package services

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"sort"
	"time"

	"delivery-system/internal/config"
	"delivery-system/internal/database"
	"delivery-system/internal/logger"
	"delivery-system/internal/models"

	"github.com/google/uuid"
)

// AssignmentService представляет сервис автоназначения курьеров
type AssignmentService struct {
	db            *database.DB
	log           *logger.Logger
	config        *config.AssignmentConfig
	orderService  *OrderService
	courierService *CourierService
}

// NewAssignmentService создает новый экземпляр сервиса автоназначения
func NewAssignmentService(
	db *database.DB,
	log *logger.Logger,
	cfg *config.AssignmentConfig,
	orderService *OrderService,
	courierService *CourierService,
) *AssignmentService {
	return &AssignmentService{
		db:             db,
		log:            log,
		config:         cfg,
		orderService:   orderService,
		courierService: courierService,
	}
}

// Ошибки сервиса
var (
	ErrNoAvailableCouriers = errors.New("no available couriers found")
	ErrOrderNotFound       = errors.New("order not found")
	ErrOrderAlreadyAssigned = errors.New("order already assigned to courier")
	ErrInvalidOrderStatus  = errors.New("order status does not allow assignment")
)

// AutoAssignCourier автоматически назначает оптимального курьера на заказ
func (s *AssignmentService) AutoAssignCourier(ctx context.Context, orderID uuid.UUID) (*models.AutoAssignmentResponse, error) {
	startTime := time.Now()
	
	s.log.Info("Starting auto-assignment", "order_id", orderID)

	// 1. Получаем заказ и проверяем его статус
	order, err := s.orderService.GetOrder(orderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrOrderNotFound
		}
		return nil, fmt.Errorf("failed to get order: %w", err)
	}

	// 2. Проверяем, можно ли назначить курьера
	if order.CourierID != nil {
		return nil, ErrOrderAlreadyAssigned
	}

	if order.Status != models.OrderStatusCreated && order.Status != models.OrderStatusReady {
		return nil, ErrInvalidOrderStatus
	}

	// 3. Находим оптимального курьера
	bestCourier, allScores, err := s.findOptimalCourier(ctx)
	if err != nil {
		if errors.Is(err, ErrNoAvailableCouriers) {
			return &models.AutoAssignmentResponse{
				Success:         false,
				OrderID:         orderID,
				Message:         "No available couriers at the moment",
				Timestamp:       time.Now(),
				TotalCandidates: 0,
			}, nil
		}
		return nil, fmt.Errorf("failed to find optimal courier: %w", err)
	}

	// 4. Назначаем курьера на заказ
	err = s.assignCourierToOrder(ctx, orderID, bestCourier.Courier.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to assign courier to order: %w", err)
	}

	// 5. Определяем причину выбора
	reason := s.determineAssignmentReason(bestCourier, allScores)

	// 6. Логируем назначение
	s.logAssignment(orderID, bestCourier, reason, len(allScores))

	// 7. Формируем ответ
	response := &models.AutoAssignmentResponse{
		Success:         true,
		OrderID:         orderID,
		AssignedCourier: bestCourier,
		Message:         fmt.Sprintf("Courier %s assigned successfully (reason: %s)", bestCourier.Courier.Name, reason),
		Timestamp:       time.Now(),
		TotalCandidates: len(allScores),
		TopCandidates:   s.getTopCandidates(allScores, 3),
	}

	duration := time.Since(startTime)
	s.log.Info("Auto-assignment completed",
		"order_id", orderID,
		"courier_id", bestCourier.Courier.ID,
		"courier_name", bestCourier.Courier.Name,
		"score", bestCourier.TotalScore,
		"duration_ms", duration.Milliseconds(),
	)

	return response, nil
}

// findOptimalCourier находит оптимального курьера по scoring алгоритму
func (s *AssignmentService) findOptimalCourier(ctx context.Context) (*models.CourierScore, []models.CourierScore, error) {
	// Получаем всех доступных курьеров
	couriers, err := s.courierService.GetAvailableCouriers()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get available couriers: %w", err)
	}

	if len(couriers) == 0 {
		// Если нет доступных, пробуем найти наименее загруженного среди занятых
		return s.findLeastBusyCourier(ctx)
	}

	// Оцениваем каждого курьера
	var scores []models.CourierScore
	for _, courier := range couriers {
		score, err := s.scoreCourier(ctx, courier)
		if err != nil {
			s.log.Warn("Failed to score courier", "courier_id", courier.ID, "error", err)
			continue
		}

		// Фильтруем по максимальному расстоянию
		if score.DistanceKm > s.config.MaxDistanceKm {
			s.log.Debug("Courier too far", "courier_id", courier.ID, "distance_km", score.DistanceKm)
			continue
		}

		// Фильтруем по максимальной загрузке
		if score.ActiveOrders >= s.config.MaxActiveOrders {
			s.log.Debug("Courier overloaded", "courier_id", courier.ID, "active_orders", score.ActiveOrders)
			continue
		}

		scores = append(scores, *score)
	}

	if len(scores) == 0 {
		return nil, nil, ErrNoAvailableCouriers
	}

	// Сортируем по итоговой оценке (от большего к меньшему)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].TotalScore > scores[j].TotalScore
	})

	return &scores[0], scores, nil
}

// findLeastBusyCourier находит наименее загруженного курьера среди всех (fallback)
func (s *AssignmentService) findLeastBusyCourier(ctx context.Context) (*models.CourierScore, []models.CourierScore, error) {
	s.log.Info("No available couriers, looking for least busy courier")

	// Получаем всех курьеров (не только available)
	allCouriers, err := s.courierService.GetCouriers(nil, 0, 0)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get all couriers: %w", err)
	}

	if len(allCouriers) == 0 {
		return nil, nil, ErrNoAvailableCouriers
	}

	var scores []models.CourierScore
	for _, courier := range allCouriers {
		if courier.Status == models.CourierStatusOffline {
			continue // Пропускаем offline курьеров
		}

		score, err := s.scoreCourier(ctx, courier)
		if err != nil {
			s.log.Warn("Failed to score courier", "courier_id", courier.ID, "error", err)
			continue
		}

		scores = append(scores, *score)
	}

	if len(scores) == 0 {
		return nil, nil, ErrNoAvailableCouriers
	}

	// Сортируем по загруженности (меньше активных заказов - лучше)
	sort.Slice(scores, func(i, j int) bool {
		return scores[i].ActiveOrders < scores[j].ActiveOrders
	})

	return &scores[0], scores, nil
}

// scoreCourier вычисляет оценку курьера
func (s *AssignmentService) scoreCourier(ctx context.Context, courier *models.Courier) (*models.CourierScore, error) {
	score := &models.CourierScore{
		Courier: courier,
	}

	// 1. Вычисляем расстояние
	if courier.CurrentLat != nil && courier.CurrentLon != nil {
		score.DistanceKm = s.calculateDistance(
			s.config.RestaurantLat, s.config.RestaurantLon,
			*courier.CurrentLat, *courier.CurrentLon,
		)
	} else {
		// Если нет координат, считаем что курьер далеко
		score.DistanceKm = s.config.MaxDistanceKm
	}
	score.DistanceScore = s.calculateDistanceScore(score.DistanceKm)

	// 2. Получаем рейтинг курьера
	rating, err := s.getCourierRating(ctx, courier.ID)
	if err != nil {
		s.log.Warn("Failed to get courier rating", "courier_id", courier.ID, "error", err)
		rating = 0 // Если нет рейтинга, ставим 0
	}
	score.RatingScore = s.calculateRatingScore(rating)

	// 3. Получаем количество активных заказов
	activeOrders, err := s.getActiveOrdersCount(ctx, courier.ID)
	if err != nil {
		s.log.Warn("Failed to get active orders count", "courier_id", courier.ID, "error", err)
		activeOrders = 0
	}
	score.ActiveOrders = activeOrders
	score.LoadScore = s.calculateLoadScore(activeOrders)

	// 4. Вычисляем итоговую оценку (взвешенная сумма)
	score.TotalScore = (score.DistanceScore * s.config.DistanceWeight) +
		(score.RatingScore * s.config.RatingWeight) +
		(score.LoadScore * s.config.LoadWeight)

	return score, nil
}

// calculateDistance вычисляет расстояние между двумя точками по формуле Haversine (в км)
func (s *AssignmentService) calculateDistance(lat1, lon1, lat2, lon2 float64) float64 {
	const earthRadiusKm = 6371.0

	// Конвертируем градусы в радианы
	lat1Rad := lat1 * math.Pi / 180
	lat2Rad := lat2 * math.Pi / 180
	deltaLat := (lat2 - lat1) * math.Pi / 180
	deltaLon := (lon2 - lon1) * math.Pi / 180

	// Формула Haversine
	a := math.Sin(deltaLat/2)*math.Sin(deltaLat/2) +
		math.Cos(lat1Rad)*math.Cos(lat2Rad)*
			math.Sin(deltaLon/2)*math.Sin(deltaLon/2)

	c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))

	distance := earthRadiusKm * c
	return math.Round(distance*100) / 100 // Округляем до 2 знаков
}

// calculateDistanceScore вычисляет оценку расстояния (0-1, меньше расстояние - больше оценка)
func (s *AssignmentService) calculateDistanceScore(distanceKm float64) float64 {
	if distanceKm >= s.config.MaxDistanceKm {
		return 0.0
	}
	// Линейная инверсия: 0 км = 1.0, MaxDistance км = 0.0
	score := 1.0 - (distanceKm / s.config.MaxDistanceKm)
	return math.Max(0, math.Min(1, score))
}

// calculateRatingScore вычисляет оценку рейтинга (0-1)
func (s *AssignmentService) calculateRatingScore(rating float64) float64 {
	// Рейтинг от 0 до 5, нормализуем к 0-1
	score := rating / 5.0
	return math.Max(0, math.Min(1, score))
}

// calculateLoadScore вычисляет оценку загруженности (0-1, меньше заказов - больше оценка)
func (s *AssignmentService) calculateLoadScore(activeOrders int) float64 {
	if activeOrders >= s.config.MaxActiveOrders {
		return 0.0
	}
	// Линейная инверсия: 0 заказов = 1.0, MaxActive заказов = 0.0
	score := 1.0 - (float64(activeOrders) / float64(s.config.MaxActiveOrders))
	return math.Max(0, math.Min(1, score))
}

// getCourierRating получает средний рейтинг курьера
func (s *AssignmentService) getCourierRating(ctx context.Context, courierID uuid.UUID) (float64, error) {
	var avgRating sql.NullFloat64
	
	query := `
		SELECT AVG(rating) as avg_rating
		FROM reviews
		WHERE courier_id = $1
	`
	
	err := s.db.QueryRowContext(ctx, query, courierID).Scan(&avgRating)
	if err != nil && err != sql.ErrNoRows {
		return 0, err
	}
	
	if avgRating.Valid {
		return avgRating.Float64, nil
	}
	
	return 0, nil // Нет отзывов - рейтинг 0
}

// getActiveOrdersCount получает количество активных заказов курьера
func (s *AssignmentService) getActiveOrdersCount(ctx context.Context, courierID uuid.UUID) (int, error) {
	var count int
	
	query := `
		SELECT COUNT(*)
		FROM orders
		WHERE courier_id = $1
		  AND status IN ('accepted', 'preparing', 'ready', 'in_delivery')
	`
	
	err := s.db.QueryRowContext(ctx, query, courierID).Scan(&count)
	if err != nil {
		return 0, err
	}
	
	return count, nil
}

// assignCourierToOrder назначает курьера на заказ
func (s *AssignmentService) assignCourierToOrder(ctx context.Context, orderID, courierID uuid.UUID) error {
	query := `
		UPDATE orders
		SET courier_id = $1, status = 'accepted', updated_at = NOW()
		WHERE id = $2
	`
	
	_, err := s.db.ExecContext(ctx, query, courierID, orderID)
	if err != nil {
		return fmt.Errorf("failed to update order: %w", err)
	}
	
	return nil
}

// determineAssignmentReason определяет причину выбора курьера
func (s *AssignmentService) determineAssignmentReason(chosen *models.CourierScore, allScores []models.CourierScore) models.AssignmentReason {
	if len(allScores) == 1 {
		return models.AssignmentReasonOnlyAvailable
	}

	if chosen.ActiveOrders > 0 {
		return models.AssignmentReasonFallback
	}

	// Проверяем, лучший ли по расстоянию
	isClosest := true
	for _, score := range allScores {
		if score.Courier.ID != chosen.Courier.ID && score.DistanceKm < chosen.DistanceKm {
			isClosest = false
			break
		}
	}

	if isClosest && chosen.DistanceScore > 0.8 {
		return models.AssignmentReasonClosest
	}

	if chosen.RatingScore > 0.9 {
		return models.AssignmentReasonBestRating
	}

	if chosen.LoadScore == 1.0 {
		return models.AssignmentReasonLeastBusy
	}

	return models.AssignmentReasonOptimal
}

// logAssignment логирует назначение курьера
func (s *AssignmentService) logAssignment(orderID uuid.UUID, chosen *models.CourierScore, reason models.AssignmentReason, totalCandidates int) {
	logEntry := models.AssignmentLog{
		OrderID:         orderID,
		CourierID:       chosen.Courier.ID,
		Reason:          reason,
		Score:           chosen.TotalScore,
		DistanceKm:      chosen.DistanceKm,
		ActiveOrders:    chosen.ActiveOrders,
		Timestamp:       time.Now(),
		TotalCandidates: totalCandidates,
	}

	s.log.Info("Courier assigned",
		"log_entry", logEntry,
		"courier_name", chosen.Courier.Name,
		"distance_score", chosen.DistanceScore,
		"rating_score", chosen.RatingScore,
		"load_score", chosen.LoadScore,
	)
}

// getTopCandidates возвращает топ N кандидатов
func (s *AssignmentService) getTopCandidates(scores []models.CourierScore, n int) []models.CourierScore {
	if len(scores) <= n {
		return scores
	}
	return scores[:n]
}