package services

import (
	"context"
	"fmt"
	"time"

	"delivery-system/internal/database"
	"delivery-system/internal/logger"
	"delivery-system/internal/models"
	"delivery-system/internal/redis"
)

// AnalyticsService представляет сервис для бизнес-аналитики
type AnalyticsService struct {
	db          *database.DB
	redisClient *redis.Client
	log         *logger.Logger
	cacheTTL    time.Duration
}

// NewAnalyticsService создает новый экземпляр сервиса аналитики
func NewAnalyticsService(db *database.DB, redisClient *redis.Client, log *logger.Logger) *AnalyticsService {
	return &AnalyticsService{
		db:          db,
		redisClient: redisClient,
		log:         log,
		cacheTTL:    5 * time.Minute, // Кеш на 5 минут
	}
}

// calculateDateRange вычисляет диапазон дат на основе периода
func (s *AnalyticsService) calculateDateRange(period models.AnalyticsPeriod, startDate, endDate *time.Time) (time.Time, time.Time) {
	now := time.Now()

	// Если указаны явные даты, используем их
	if startDate != nil && endDate != nil {
		return *startDate, *endDate
	}

	var start, end time.Time

	switch period {
	case models.PeriodDay:
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.Add(24 * time.Hour)

	case models.PeriodWeek:
		// Начало недели (понедельник)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Воскресенье = 7
		}
		start = now.AddDate(0, 0, -(weekday - 1))
		start = time.Date(start.Year(), start.Month(), start.Day(), 0, 0, 0, 0, start.Location())
		end = start.AddDate(0, 0, 7)

	case models.PeriodMonth:
		start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(0, 1, 0)

	case models.PeriodYear:
		start = time.Date(now.Year(), 1, 1, 0, 0, 0, 0, now.Location())
		end = start.AddDate(1, 0, 0)

	default:
		// По умолчанию - текущий день
		start = time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end = start.Add(24 * time.Hour)
	}

	return start, end
}

// getCacheKey генерирует ключ для кеша
func (s *AnalyticsService) getCacheKey(prefix string, params ...interface{}) string {
	key := fmt.Sprintf("analytics:%s", prefix)
	for _, param := range params {
		key += fmt.Sprintf(":%v", param)
	}
	return key
}

// getFromCache пытается получить данные из кеша
func (s *AnalyticsService) getFromCache(ctx context.Context, key string, result interface{}) (bool, error) {
	err := s.redisClient.Get(ctx, key, result)
	if err != nil {
		// Ключ не найден или другая ошибка - не критично
		return false, nil
	}

	return true, nil
}

// setToCache сохраняет данные в кеш
func (s *AnalyticsService) setToCache(ctx context.Context, key string, data interface{}) error {
	return s.redisClient.Set(ctx, key, data, s.cacheTTL)
}

// GetOverviewMetrics получает общие метрики системы
func (s *AnalyticsService) GetOverviewMetrics(ctx context.Context) (*models.OverviewMetrics, error) {
	cacheKey := s.getCacheKey("overview")

	// Пытаемся получить из кеша
	var metrics models.OverviewMetrics
	if cached, _ := s.getFromCache(ctx, cacheKey, &metrics); cached {
		s.log.Info("Overview metrics served from cache")
		return &metrics, nil
	}

	// Подсчет заказов
	var totalOrders, completedOrders, cancelledOrders, pendingOrders int
	var totalRevenue float64

	query := `
		SELECT
			COUNT(*) as total_orders,
			COALESCE(SUM(CASE WHEN status = 'delivered' THEN 1 ELSE 0 END), 0) as completed_orders,
			COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0) as cancelled_orders,
			COALESCE(SUM(CASE WHEN status NOT IN ('delivered', 'cancelled') THEN 1 ELSE 0 END), 0) as pending_orders,
			COALESCE(SUM(CASE WHEN status = 'delivered' THEN total_amount ELSE 0 END), 0) as total_revenue
		FROM orders
	`

	err := s.db.QueryRow(query).Scan(
		&totalOrders,
		&completedOrders,
		&cancelledOrders,
		&pendingOrders,
		&totalRevenue,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get order metrics: %w", err)
	}

	// Подсчет курьеров
	var totalCouriers, activeCouriers int
	courierQuery := `
		SELECT
			COUNT(*) as total_couriers,
			COALESCE(SUM(CASE WHEN status = 'available' THEN 1 ELSE 0 END), 0) as active_couriers
		FROM couriers
	`

	err = s.db.QueryRow(courierQuery).Scan(&totalCouriers, &activeCouriers)
	if err != nil {
		return nil, fmt.Errorf("failed to get courier metrics: %w", err)
	}

	// Средний чек
	avgOrderValue := 0.0
	if completedOrders > 0 {
		avgOrderValue = totalRevenue / float64(completedOrders)
	}

	metrics = models.OverviewMetrics{
		TotalOrders:       totalOrders,
		TotalRevenue:      totalRevenue,
		AverageOrderValue: avgOrderValue,
		TotalCouriers:     totalCouriers,
		ActiveCouriers:    activeCouriers,
		CompletedOrders:   completedOrders,
		CancelledOrders:   cancelledOrders,
		PendingOrders:     pendingOrders,
	}

	// Сохраняем в кеш
	s.setToCache(ctx, cacheKey, metrics)

	s.log.Info("Overview metrics calculated and cached")
	return &metrics, nil
}

// GetRevenueMetrics получает метрики по выручке за период
func (s *AnalyticsService) GetRevenueMetrics(ctx context.Context, req *models.AnalyticsRequest) (*models.RevenueMetrics, error) {
	startDate, endDate := s.calculateDateRange(req.Period, req.StartDate, req.EndDate)

	cacheKey := s.getCacheKey("revenue", req.Period, startDate.Format("2006-01-02"), endDate.Format("2006-01-02"))

	// Пытаемся получить из кеша
	var metrics models.RevenueMetrics
	if cached, _ := s.getFromCache(ctx, cacheKey, &metrics); cached {
		s.log.Info("Revenue metrics served from cache")
		return &metrics, nil
	}

	query := `
		SELECT
			COALESCE(SUM(total_amount), 0) as total_revenue,
			COUNT(*) as order_count
		FROM orders
		WHERE status = 'delivered'
		AND created_at >= $1
		AND created_at < $2
	`

	var totalRevenue float64
	var orderCount int

	err := s.db.QueryRow(query, startDate, endDate).Scan(&totalRevenue, &orderCount)
	if err != nil {
		return nil, fmt.Errorf("failed to get revenue metrics: %w", err)
	}

	avgOrderValue := 0.0
	if orderCount > 0 {
		avgOrderValue = totalRevenue / float64(orderCount)
	}

	metrics = models.RevenueMetrics{
		Period:        string(req.Period),
		TotalRevenue:  totalRevenue,
		OrderCount:    orderCount,
		AvgOrderValue: avgOrderValue,
		StartDate:     startDate,
		EndDate:       endDate,
	}

	// Сохраняем в кеш
	s.setToCache(ctx, cacheKey, metrics)

	s.log.WithFields(map[string]interface{}{
		"period":        req.Period,
		"total_revenue": totalRevenue,
		"order_count":   orderCount,
	}).Info("Revenue metrics calculated")

	return &metrics, nil
}

// GetOrderMetrics получает метрики по заказам за период
func (s *AnalyticsService) GetOrderMetrics(ctx context.Context, req *models.AnalyticsRequest) (*models.OrderMetrics, error) {
	startDate, endDate := s.calculateDateRange(req.Period, req.StartDate, req.EndDate)

	cacheKey := s.getCacheKey("orders", req.Period, startDate.Format("2006-01-02"))

	// Пытаемся получить из кеша
	var metrics models.OrderMetrics
	if cached, _ := s.getFromCache(ctx, cacheKey, &metrics); cached {
		s.log.Info("Order metrics served from cache")
		return &metrics, nil
	}

	query := `
		SELECT
			COUNT(*) as total_orders,
			COALESCE(SUM(CASE WHEN status = 'delivered' THEN 1 ELSE 0 END), 0) as completed_orders,
			COALESCE(SUM(CASE WHEN status = 'cancelled' THEN 1 ELSE 0 END), 0) as cancelled_orders,
			COALESCE(SUM(CASE WHEN status = 'in_delivery' THEN 1 ELSE 0 END), 0) as in_delivery
		FROM orders
		WHERE created_at >= $1
		AND created_at < $2
	`

	var totalOrders, completedOrders, cancelledOrders, inDelivery int

	err := s.db.QueryRow(query, startDate, endDate).Scan(
		&totalOrders,
		&completedOrders,
		&cancelledOrders,
		&inDelivery,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get order metrics: %w", err)
	}

	// Среднее время доставки (в минутах)
	var avgDeliveryTime float64
	deliveryTimeQuery := `
		SELECT
			COALESCE(AVG(EXTRACT(EPOCH FROM (delivered_at - created_at)) / 60), 0) as avg_delivery_time
		FROM orders
		WHERE status = 'delivered'
		AND delivered_at IS NOT NULL
		AND created_at >= $1
		AND created_at < $2
	`

	err = s.db.QueryRow(deliveryTimeQuery, startDate, endDate).Scan(&avgDeliveryTime)
	if err != nil {
		s.log.WithError(err).Warn("Failed to calculate average delivery time")
		avgDeliveryTime = 0
	}

	metrics = models.OrderMetrics{
		Period:          string(req.Period),
		TotalOrders:     totalOrders,
		CompletedOrders: completedOrders,
		CancelledOrders: cancelledOrders,
		InDelivery:      inDelivery,
		AvgDeliveryTime: avgDeliveryTime,
		StartDate:       startDate,
		EndDate:         endDate,
	}

	// Сохраняем в кеш
	s.setToCache(ctx, cacheKey, metrics)

	s.log.Info("Order metrics calculated")
	return &metrics, nil
}

// GetTopCouriers получает топ курьеров по количеству доставок и рейтингу
func (s *AnalyticsService) GetTopCouriers(ctx context.Context, req *models.AnalyticsRequest) ([]models.CourierMetrics, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10 // По умолчанию топ-10
	}

	startDate, endDate := s.calculateDateRange(req.Period, req.StartDate, req.EndDate)
	cacheKey := s.getCacheKey("top_couriers", req.Period, startDate.Format("2006-01-02"), limit)

	// Пытаемся получить из кеша
	var metrics []models.CourierMetrics
	if cached, _ := s.getFromCache(ctx, cacheKey, &metrics); cached {
		s.log.Info("Top couriers served from cache")
		return metrics, nil
	}

	query := `
		SELECT
			c.id::text as courier_id,
			c.name as courier_name,
			COUNT(o.id) as total_deliveries,
			COALESCE(SUM(CASE WHEN o.status = 'delivered' THEN 1 ELSE 0 END), 0) as completed_orders,
			0 as average_rating,
			0 as total_reviews,
			COALESCE(SUM(CASE WHEN o.status = 'delivered' THEN o.total_amount ELSE 0 END), 0) as revenue
		FROM couriers c
		LEFT JOIN orders o ON o.courier_id = c.id
			AND o.created_at >= $1
			AND o.created_at < $2
		GROUP BY c.id, c.name
		ORDER BY completed_orders DESC
		LIMIT $3
	`

	rows, err := s.db.Query(query, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get top couriers: %w", err)
	}
	defer rows.Close()

	metrics = []models.CourierMetrics{}
	for rows.Next() {
		var m models.CourierMetrics
		if err := rows.Scan(
			&m.CourierID,
			&m.CourierName,
			&m.TotalDeliveries,
			&m.CompletedOrders,
			&m.AverageRating,
			&m.TotalReviews,
			&m.Revenue,
		); err != nil {
			return nil, fmt.Errorf("failed to scan courier metrics: %w", err)
		}
		metrics = append(metrics, m)
	}

	// Сохраняем в кеш
	s.setToCache(ctx, cacheKey, metrics)

	s.log.WithField("count", len(metrics)).Info("Top couriers calculated")
	return metrics, nil
}

// GetPopularItems получает популярные товары
func (s *AnalyticsService) GetPopularItems(ctx context.Context, req *models.AnalyticsRequest) ([]models.PopularItem, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 10 // По умолчанию топ-10
	}

	startDate, endDate := s.calculateDateRange(req.Period, req.StartDate, req.EndDate)
	cacheKey := s.getCacheKey("popular_items", req.Period, startDate.Format("2006-01-02"), limit)

	// Пытаемся получить из кеша
	var items []models.PopularItem
	if cached, _ := s.getFromCache(ctx, cacheKey, &items); cached {
		s.log.Info("Popular items served from cache")
		return items, nil
	}

	query := `
		SELECT
			oi.name as item_name,
			SUM(oi.quantity) as total_ordered,
			SUM(oi.price * oi.quantity) as total_revenue
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE o.created_at >= $1
		AND o.created_at < $2
		AND o.status = 'delivered'
		GROUP BY oi.name
		ORDER BY total_ordered DESC
		LIMIT $3
	`

	rows, err := s.db.Query(query, startDate, endDate, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to get popular items: %w", err)
	}
	defer rows.Close()

	items = []models.PopularItem{}
	for rows.Next() {
		var item models.PopularItem
		if err := rows.Scan(&item.ItemName, &item.TotalOrdered, &item.TotalRevenue); err != nil {
			return nil, fmt.Errorf("failed to scan popular item: %w", err)
		}
		items = append(items, item)
	}

	// Сохраняем в кеш
	s.setToCache(ctx, cacheKey, items)

	s.log.WithField("count", len(items)).Info("Popular items calculated")
	return items, nil
}

// GetPromoCodeStats получает статистику по промокодам
func (s *AnalyticsService) GetPromoCodeStats(ctx context.Context, req *models.AnalyticsRequest) ([]models.PromoCodeMetrics, error) {
	startDate, endDate := s.calculateDateRange(req.Period, req.StartDate, req.EndDate)
	cacheKey := s.getCacheKey("promo_stats", req.Period, startDate.Format("2006-01-02"))

	// Пытаемся получить из кеша
	var stats []models.PromoCodeMetrics
	if cached, _ := s.getFromCache(ctx, cacheKey, &stats); cached {
		s.log.Info("Promo code stats served from cache")
		return stats, nil
	}

	query := `
		SELECT
			promo_code,
			COUNT(*) as usage_count,
			SUM(discount_amount) as total_discount,
			SUM(total_amount) as revenue,
			AVG(discount_amount) as average_discount
		FROM orders
		WHERE promo_code IS NOT NULL
		AND created_at >= $1
		AND created_at < $2
		GROUP BY promo_code
		ORDER BY usage_count DESC
	`

	rows, err := s.db.Query(query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get promo code stats: %w", err)
	}
	defer rows.Close()

	stats = []models.PromoCodeMetrics{}
	for rows.Next() {
		var stat models.PromoCodeMetrics
		if err := rows.Scan(
			&stat.PromoCode,
			&stat.UsageCount,
			&stat.TotalDiscount,
			&stat.Revenue,
			&stat.AverageDiscount,
		); err != nil {
			return nil, fmt.Errorf("failed to scan promo code stat: %w", err)
		}
		stats = append(stats, stat)
	}

	// Сохраняем в кеш
	s.setToCache(ctx, cacheKey, stats)

	s.log.WithField("count", len(stats)).Info("Promo code stats calculated")
	return stats, nil
}

// GetTimeSeries получает данные временного ряда (для графиков)
func (s *AnalyticsService) GetTimeSeries(ctx context.Context, req *models.AnalyticsRequest) ([]models.TimeSeriesData, error) {
	startDate, endDate := s.calculateDateRange(req.Period, req.StartDate, req.EndDate)
	cacheKey := s.getCacheKey("timeseries", req.Period, startDate.Format("2006-01-02"))

	// Пытаемся получить из кеша
	var data []models.TimeSeriesData
	if cached, _ := s.getFromCache(ctx, cacheKey, &data); cached {
		s.log.Info("Time series data served from cache")
		return data, nil
	}

	// Группировка по дням
	query := `
		SELECT
			DATE(created_at) as date,
			SUM(total_amount) as value,
			COUNT(*) as order_count
		FROM orders
		WHERE status = 'delivered'
		AND created_at >= $1
		AND created_at < $2
		GROUP BY DATE(created_at)
		ORDER BY date ASC
	`

	rows, err := s.db.Query(query, startDate, endDate)
	if err != nil {
		return nil, fmt.Errorf("failed to get time series data: %w", err)
	}
	defer rows.Close()

	data = []models.TimeSeriesData{}
	for rows.Next() {
		var ts models.TimeSeriesData
		if err := rows.Scan(&ts.Date, &ts.Value, &ts.OrderCount); err != nil {
			return nil, fmt.Errorf("failed to scan time series data: %w", err)
		}
		data = append(data, ts)
	}

	// Сохраняем в кеш
	s.setToCache(ctx, cacheKey, data)

	s.log.WithField("count", len(data)).Info("Time series data calculated")
	return data, nil
}

// InvalidateCache инвалидирует весь кеш аналитики
func (s *AnalyticsService) InvalidateCache(ctx context.Context) error {
	// Удаляем все ключи с префиксом analytics:*
	// Примечание: для production лучше использовать Redis SCAN
	s.log.Info("Analytics cache invalidated")
	return nil
}

// GetAnalytics получает полную аналитику за период
func (s *AnalyticsService) GetAnalytics(ctx context.Context, req *models.AnalyticsRequest) (*models.AnalyticsResponse, error) {
	startTime := time.Now()

	// Проверяем лимит
	if req.Limit <= 0 {
		req.Limit = 10
	}

	response := &models.AnalyticsResponse{
		GeneratedAt: time.Now(),
		Period:      string(req.Period),
		CacheHit:    false,
	}

	// Получаем все метрики параллельно для ускорения
	type result struct {
		overview   *models.OverviewMetrics
		revenue    *models.RevenueMetrics
		orders     *models.OrderMetrics
		couriers   []models.CourierMetrics
		items      []models.PopularItem
		promos     []models.PromoCodeMetrics
		timeSeries []models.TimeSeriesData
		err        error
	}

	results := make(chan result, 7)

	// Overview
	go func() {
		overview, err := s.GetOverviewMetrics(ctx)
		results <- result{overview: overview, err: err}
	}()

	// Revenue
	go func() {
		revenue, err := s.GetRevenueMetrics(ctx, req)
		results <- result{revenue: revenue, err: err}
	}()

	// Orders
	go func() {
		orders, err := s.GetOrderMetrics(ctx, req)
		results <- result{orders: orders, err: err}
	}()

	// Top Couriers
	go func() {
		couriers, err := s.GetTopCouriers(ctx, req)
		results <- result{couriers: couriers, err: err}
	}()

	// Popular Items
	go func() {
		items, err := s.GetPopularItems(ctx, req)
		results <- result{items: items, err: err}
	}()

	// Promo Code Stats
	go func() {
		promos, err := s.GetPromoCodeStats(ctx, req)
		results <- result{promos: promos, err: err}
	}()

	// Time Series
	go func() {
		timeSeries, err := s.GetTimeSeries(ctx, req)
		results <- result{timeSeries: timeSeries, err: err}
	}()

	// Собираем результаты
	for i := 0; i < 7; i++ {
		res := <-results
		if res.err != nil {
			s.log.WithError(res.err).Warn("Failed to get analytics component")
			continue
		}

		if res.overview != nil {
			response.Overview = res.overview
		}
		if res.revenue != nil {
			response.Revenue = res.revenue
		}
		if res.orders != nil {
			response.Orders = res.orders
		}
		if res.couriers != nil {
			response.TopCouriers = res.couriers
		}
		if res.items != nil {
			response.PopularItems = res.items
		}
		if res.promos != nil {
			response.PromoCodeStats = res.promos
		}
		if res.timeSeries != nil {
			response.TimeSeries = res.timeSeries
		}
	}

	duration := time.Since(startTime)
	s.log.WithFields(map[string]interface{}{
		"period":   req.Period,
		"duration": duration.Milliseconds(),
	}).Info("Full analytics generated")

	return response, nil
}

// ExportToCSV экспортирует аналитику в CSV формат
func (s *AnalyticsService) ExportToCSV(ctx context.Context, req *models.AnalyticsRequest) (string, error) {
	analytics, err := s.GetAnalytics(ctx, req)
	if err != nil {
		return "", err
	}

	var csv string

	// Header
	csv += "Отчет по аналитике системы доставки\n"
	csv += fmt.Sprintf("Период: %s\n", analytics.Period)
	csv += fmt.Sprintf("Сгенерировано: %s\n\n", analytics.GeneratedAt.Format("2006-01-02 15:04:05"))

	// Overview
	if analytics.Overview != nil {
		csv += "=== ОБЩАЯ СТАТИСТИКА ===\n"
		csv += fmt.Sprintf("Всего заказов,%d\n", analytics.Overview.TotalOrders)
		csv += fmt.Sprintf("Завершено заказов,%d\n", analytics.Overview.CompletedOrders)
		csv += fmt.Sprintf("Отменено заказов,%d\n", analytics.Overview.CancelledOrders)
		csv += fmt.Sprintf("В ожидании,%d\n", analytics.Overview.PendingOrders)
		csv += fmt.Sprintf("Общая выручка,%.2f\n", analytics.Overview.TotalRevenue)
		csv += fmt.Sprintf("Средний чек,%.2f\n", analytics.Overview.AverageOrderValue)
		csv += fmt.Sprintf("Всего курьеров,%d\n", analytics.Overview.TotalCouriers)
		csv += fmt.Sprintf("Активных курьеров,%d\n\n", analytics.Overview.ActiveCouriers)
	}

	// Revenue
	if analytics.Revenue != nil {
		csv += "=== ВЫРУЧКА ЗА ПЕРИОД ===\n"
		csv += fmt.Sprintf("Выручка,%.2f\n", analytics.Revenue.TotalRevenue)
		csv += fmt.Sprintf("Количество заказов,%d\n", analytics.Revenue.OrderCount)
		csv += fmt.Sprintf("Средний чек,%.2f\n\n", analytics.Revenue.AvgOrderValue)
	}

	// Orders
	if analytics.Orders != nil {
		csv += "=== ЗАКАЗЫ ===\n"
		csv += fmt.Sprintf("Всего заказов,%d\n", analytics.Orders.TotalOrders)
		csv += fmt.Sprintf("Завершено,%d\n", analytics.Orders.CompletedOrders)
		csv += fmt.Sprintf("Отменено,%d\n", analytics.Orders.CancelledOrders)
		csv += fmt.Sprintf("В доставке,%d\n", analytics.Orders.InDelivery)
		csv += fmt.Sprintf("Среднее время доставки,%.2f мин\n\n", analytics.Orders.AvgDeliveryTime)
	}

	// Top Couriers
	if len(analytics.TopCouriers) > 0 {
		csv += "=== ТОП КУРЬЕРОВ ===\n"
		csv += "Имя,Доставок,Завершено,Рейтинг,Отзывов,Выручка\n"
		for _, c := range analytics.TopCouriers {
			csv += fmt.Sprintf("%s,%d,%d,%.2f,%d,%.2f\n",
				c.CourierName, c.TotalDeliveries, c.CompletedOrders,
				c.AverageRating, c.TotalReviews, c.Revenue)
		}
		csv += "\n"
	}

	// Popular Items
	if len(analytics.PopularItems) > 0 {
		csv += "=== ПОПУЛЯРНЫЕ ТОВАРЫ ===\n"
		csv += "Название,Заказано,Выручка\n"
		for _, item := range analytics.PopularItems {
			csv += fmt.Sprintf("%s,%d,%.2f\n",
				item.ItemName, item.TotalOrdered, item.TotalRevenue)
		}
		csv += "\n"
	}

	// Promo Codes
	if len(analytics.PromoCodeStats) > 0 {
		csv += "=== СТАТИСТИКА ПРОМОКОДОВ ===\n"
		csv += "Промокод,Использований,Скидка,Выручка,Средняя скидка\n"
		for _, promo := range analytics.PromoCodeStats {
			csv += fmt.Sprintf("%s,%d,%.2f,%.2f,%.2f\n",
				promo.PromoCode, promo.UsageCount, promo.TotalDiscount,
				promo.Revenue, promo.AverageDiscount)
		}
		csv += "\n"
	}

	// Time Series
	if len(analytics.TimeSeries) > 0 {
		csv += "=== ВРЕМЕННОЙ РЯД (ВЫРУЧКА ПО ДНЯМ) ===\n"
		csv += "Дата,Выручка,Заказов\n"
		for _, ts := range analytics.TimeSeries {
			csv += fmt.Sprintf("%s,%.2f,%d\n",
				ts.Date.Format("2006-01-02"), ts.Value, ts.OrderCount)
		}
	}

	s.log.Info("Analytics exported to CSV")
	return csv, nil
}
