package kafka

import (
	"sync/atomic"
	"time"
)

// EventMetrics представляет метрики обработки событий Kafka
type EventMetrics struct {
	// Счётчики событий по типам
	processedCount atomic.Uint64 // Успешно обработанные
	errorCount     atomic.Uint64 // Ошибки обработки
	retriedCount   atomic.Uint64 // Повторные попытки
	dlqCount       atomic.Uint64 // Отправлено в DLQ
	
	// Метрики производительности
	totalProcessingTime atomic.Uint64 // Общее время обработки (наносекунды)
	
	// Метрики по топикам (для простоты используем map, в production - sync.Map)
	topicStats map[string]*TopicStats
}

// TopicStats представляет статистику по конкретному топику
type TopicStats struct {
	Processed atomic.Uint64
	Errors    atomic.Uint64
	AvgLatencyMs float64 // Средняя задержка в миллисекундах
}

// NewEventMetrics создаёт новый экземпляр метрик
func NewEventMetrics() *EventMetrics {
	return &EventMetrics{
		topicStats: make(map[string]*TopicStats),
	}
}

// RecordProcessed записывает успешно обработанное событие
func (m *EventMetrics) RecordProcessed(topic string, duration time.Duration) {
	m.processedCount.Add(1)
	m.totalProcessingTime.Add(uint64(duration.Nanoseconds()))
	
	// Обновляем статистику по топику
	if stats, exists := m.topicStats[topic]; exists {
		stats.Processed.Add(1)
	} else {
		m.topicStats[topic] = &TopicStats{}
		m.topicStats[topic].Processed.Add(1)
	}
}

// RecordError записывает ошибку обработки
func (m *EventMetrics) RecordError(topic string) {
	m.errorCount.Add(1)
	
	if stats, exists := m.topicStats[topic]; exists {
		stats.Errors.Add(1)
	} else {
		m.topicStats[topic] = &TopicStats{}
		m.topicStats[topic].Errors.Add(1)
	}
}

// RecordRetry записывает повторную попытку
func (m *EventMetrics) RecordRetry() {
	m.retriedCount.Add(1)
}

// RecordDLQ записывает отправку в Dead Letter Queue
func (m *EventMetrics) RecordDLQ() {
	m.dlqCount.Add(1)
}

// GetStats возвращает текущую статистику
func (m *EventMetrics) GetStats() EventStatsResponse {
	processed := m.processedCount.Load()
	errors := m.errorCount.Load()
	total := processed + errors
	
	var avgProcessingMs float64
	if processed > 0 {
		totalNs := m.totalProcessingTime.Load()
		avgProcessingMs = float64(totalNs) / float64(processed) / 1e6 // наносекунды → миллисекунды
	}
	
	var errorRate float64
	if total > 0 {
		errorRate = float64(errors) / float64(total) * 100
	}
	
	// Собираем статистику по топикам
	topicStats := make(map[string]TopicStatsResponse)
	for topic, stats := range m.topicStats {
		topicStats[topic] = TopicStatsResponse{
			Processed: stats.Processed.Load(),
			Errors:    stats.Errors.Load(),
		}
	}
	
	return EventStatsResponse{
		TotalProcessed:      processed,
		TotalErrors:         errors,
		TotalRetried:        m.retriedCount.Load(),
		TotalDLQ:            m.dlqCount.Load(),
		AvgProcessingTimeMs: avgProcessingMs,
		ErrorRate:           errorRate,
		TopicStats:          topicStats,
	}
}

// EventStatsResponse представляет ответ API со статистикой
type EventStatsResponse struct {
	TotalProcessed      uint64                       `json:"total_processed"`
	TotalErrors         uint64                       `json:"total_errors"`
	TotalRetried        uint64                       `json:"total_retried"`
	TotalDLQ            uint64                       `json:"total_dlq"`
	AvgProcessingTimeMs float64                      `json:"avg_processing_time_ms"`
	ErrorRate           float64                      `json:"error_rate"`
	TopicStats          map[string]TopicStatsResponse `json:"topic_stats"`
}

// TopicStatsResponse представляет статистику по топику для API
type TopicStatsResponse struct {
	Processed uint64 `json:"processed"`
	Errors    uint64 `json:"errors"`
}