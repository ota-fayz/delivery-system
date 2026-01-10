package models

import "time"

// AnalyticsPeriod представляет период для аналитики
type AnalyticsPeriod string

const (
	PeriodDay   AnalyticsPeriod = "day"
	PeriodWeek  AnalyticsPeriod = "week"
	PeriodMonth AnalyticsPeriod = "month"
	PeriodYear  AnalyticsPeriod = "year"
)

// OverviewMetrics представляет общие метрики системы
type OverviewMetrics struct {
	TotalOrders       int     `json:"total_orders"`
	TotalRevenue      float64 `json:"total_revenue"`
	AverageOrderValue float64 `json:"average_order_value"`
	TotalCouriers     int     `json:"total_couriers"`
	ActiveCouriers    int     `json:"active_couriers"`
	CompletedOrders   int     `json:"completed_orders"`
	CancelledOrders   int     `json:"cancelled_orders"`
	PendingOrders     int     `json:"pending_orders"`
}

// RevenueMetrics представляет метрики по выручке
type RevenueMetrics struct {
	Period        string    `json:"period"`
	TotalRevenue  float64   `json:"total_revenue"`
	OrderCount    int       `json:"order_count"`
	AvgOrderValue float64   `json:"average_order_value"`
	StartDate     time.Time `json:"start_date"`
	EndDate       time.Time `json:"end_date"`
}

// OrderMetrics представляет метрики по заказам
type OrderMetrics struct {
	Period          string    `json:"period"`
	TotalOrders     int       `json:"total_orders"`
	CompletedOrders int       `json:"completed_orders"`
	CancelledOrders int       `json:"cancelled_orders"`
	InDelivery      int       `json:"in_delivery"`
	AvgDeliveryTime float64   `json:"avg_delivery_time_minutes"` // в минутах
	StartDate       time.Time `json:"start_date"`
	EndDate         time.Time `json:"end_date"`
}

// CourierMetrics представляет метрики по курьеру
type CourierMetrics struct {
	CourierID       string  `json:"courier_id"`
	CourierName     string  `json:"courier_name"`
	TotalDeliveries int     `json:"total_deliveries"`
	CompletedOrders int     `json:"completed_orders"`
	AverageRating   float64 `json:"average_rating"`
	TotalReviews    int     `json:"total_reviews"`
	Revenue         float64 `json:"revenue"` // Выручка от доставок
}

// PopularItem представляет популярный товар
type PopularItem struct {
	ItemName     string  `json:"item_name"`
	TotalOrdered int     `json:"total_ordered"`
	TotalRevenue float64 `json:"total_revenue"`
}

// PromoCodeMetrics представляет метрики по промокодам
type PromoCodeMetrics struct {
	PromoCode       string  `json:"promo_code"`
	UsageCount      int     `json:"usage_count"`
	TotalDiscount   float64 `json:"total_discount"`
	Revenue         float64 `json:"revenue"`         // Выручка с этим промокодом
	AverageDiscount float64 `json:"average_discount"`
}

// TimeSeriesData представляет данные временного ряда
type TimeSeriesData struct {
	Date    time.Time `json:"date"`
	Value   float64   `json:"value"`
	OrderCount int    `json:"order_count,omitempty"`
}

// AnalyticsResponse представляет общий ответ аналитики
type AnalyticsResponse struct {
	Overview      *OverviewMetrics        `json:"overview,omitempty"`
	Revenue       *RevenueMetrics         `json:"revenue,omitempty"`
	Orders        *OrderMetrics           `json:"orders,omitempty"`
	TopCouriers   []CourierMetrics        `json:"top_couriers,omitempty"`
	PopularItems  []PopularItem           `json:"popular_items,omitempty"`
	PromoCodeStats []PromoCodeMetrics     `json:"promo_code_stats,omitempty"`
	TimeSeries    []TimeSeriesData        `json:"time_series,omitempty"`
	GeneratedAt   time.Time               `json:"generated_at"`
	Period        string                  `json:"period,omitempty"`
	CacheHit      bool                    `json:"cache_hit"`
}

// AnalyticsRequest представляет запрос для получения аналитики
type AnalyticsRequest struct {
	Period    AnalyticsPeriod `json:"period"`
	StartDate *time.Time      `json:"start_date,omitempty"`
	EndDate   *time.Time      `json:"end_date,omitempty"`
	Limit     int             `json:"limit,omitempty"` // Для топов (топ-10 курьеров и т.д.)
}

// ExportFormat представляет формат экспорта
type ExportFormat string

const (
	ExportJSON ExportFormat = "json"
	ExportCSV  ExportFormat = "csv"
)