package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"delivery-system/internal/config"
	"delivery-system/internal/logger"
	"delivery-system/internal/models"

	"github.com/IBM/sarama"
)

// EventHandler представляет обработчик событий
type EventHandler func(ctx context.Context, event *models.Event) error

// Consumer представляет Kafka consumer
type Consumer struct {
	consumer    sarama.ConsumerGroup
	log         *logger.Logger
	handlers    map[models.EventType]EventHandler
	topics      []string
	ctx         context.Context
	cancel      context.CancelFunc
	wg          sync.WaitGroup
	metrics     *EventMetrics
	dlqProducer *Producer
}

// NewConsumer создает новый Kafka consumer
func NewConsumer(cfg *config.KafkaConfig, log *logger.Logger, metrics *EventMetrics, dlqProducer *Producer) (*Consumer, error) {
	config := sarama.NewConfig()
	config.Consumer.Group.Rebalance.Strategy = sarama.BalanceStrategyRoundRobin
	config.Consumer.Offsets.Initial = sarama.OffsetOldest
	config.Consumer.Group.Session.Timeout = 10000000000   // 10 секунд
	config.Consumer.Group.Heartbeat.Interval = 3000000000 // 3 секунды

	consumer, err := sarama.NewConsumerGroup(cfg.Brokers, cfg.GroupID, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kafka consumer: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	topics := []string{cfg.Topics.Orders, cfg.Topics.Couriers, cfg.Topics.Locations}

	log.Info("Kafka consumer created successfully")

	return &Consumer{
		consumer:    consumer,
		log:         log,
		handlers:    make(map[models.EventType]EventHandler),
		topics:      topics,
		ctx:         ctx,
		cancel:      cancel,
		metrics:     metrics,
		dlqProducer: dlqProducer,
	}, nil
}

// RegisterHandler регистрирует обработчик для определенного типа события
func (c *Consumer) RegisterHandler(eventType models.EventType, handler EventHandler) {
	c.handlers[eventType] = handler
	c.log.WithField("event_type", eventType).Info("Event handler registered")
}

// Start запускает consumer
func (c *Consumer) Start() error {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		for {
			select {
			case <-c.ctx.Done():
				return
			default:
				if err := c.consumer.Consume(c.ctx, c.topics, c); err != nil {
					c.log.WithError(err).Error("Error consuming messages")
				}
			}
		}
	}()

	c.log.Info("Kafka consumer started")
	return nil
}

// Stop останавливает consumer
func (c *Consumer) Stop() error {
	c.cancel()
	c.wg.Wait()
	return c.consumer.Close()
}

// Setup реализует интерфейс sarama.ConsumerGroupHandler
func (c *Consumer) Setup(sarama.ConsumerGroupSession) error {
	return nil
}

// Cleanup реализует интерфейс sarama.ConsumerGroupHandler
func (c *Consumer) Cleanup(sarama.ConsumerGroupSession) error {
	return nil
}

// ConsumeClaim реализует интерфейс sarama.ConsumerGroupHandler
func (c *Consumer) ConsumeClaim(session sarama.ConsumerGroupSession, claim sarama.ConsumerGroupClaim) error {
	for {
		select {
		case message := <-claim.Messages():
			if message == nil {
				return nil
			}

			if err := c.processMessage(message); err != nil {
				c.log.WithError(err).
					WithField("topic", message.Topic).
					WithField("partition", message.Partition).
					WithField("offset", message.Offset).
					Error("Failed to process message")
			} else {
				session.MarkMessage(message, "")
			}

		case <-session.Context().Done():
			return nil
		}
	}
}

// processMessage обрабатывает полученное сообщение с middleware
func (c *Consumer) processMessage(message *sarama.ConsumerMessage) error {
	startTime := time.Now()
	
	var event models.Event
	if err := json.Unmarshal(message.Value, &event); err != nil {
		return fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Логируем начало обработки с correlation_id
	c.log.WithField("event_type", event.Type).
		WithField("event_id", event.ID).
		WithField("correlation_id", event.CorrelationID).
		WithField("retry_count", event.RetryCount).
		WithField("topic", message.Topic).
		Info("Processing event")

	// Находим обработчик для данного типа события
	handler, exists := c.handlers[event.Type]
	if !exists {
		c.log.WithField("event_type", event.Type).
			WithField("correlation_id", event.CorrelationID).
			Warn("No handler registered for event type")
		return nil
	}

	// Вызываем обработчик с retry логикой
	err := handler(c.ctx, &event)
	duration := time.Since(startTime)

	if err != nil {
		// Обработка ошибки
		c.metrics.RecordError(message.Topic)
		
		c.log.WithError(err).
			WithField("event_type", event.Type).
			WithField("event_id", event.ID).
			WithField("correlation_id", event.CorrelationID).
			WithField("retry_count", event.RetryCount).
			WithField("duration_ms", duration.Milliseconds()).
			Error("Event processing failed")

		// Проверяем, нужно ли retry
		if event.RetryCount < 3 {
			// Увеличиваем счётчик попыток и отправляем обратно в топик
			event.RetryCount++
			c.metrics.RecordRetry()
			
			c.log.WithField("event_id", event.ID).
				WithField("correlation_id", event.CorrelationID).
				WithField("retry_count", event.RetryCount).
				Warn("Retrying event processing")
			
			// Отправляем обратно в тот же топик для повторной обработки
			if retryErr := c.republishEvent(message.Topic, &event); retryErr != nil {
				c.log.WithError(retryErr).
					WithField("correlation_id", event.CorrelationID).
					Error("Failed to republish event for retry")
			}
		} else {
			// Максимум попыток достигнут - отправляем в DLQ
			c.metrics.RecordDLQ()
			
			c.log.WithField("event_id", event.ID).
				WithField("correlation_id", event.CorrelationID).
				Error("Max retries exceeded, sending to DLQ")
			
			if dlqErr := c.sendToDLQ(message.Topic, &event, err); dlqErr != nil {
				c.log.WithError(dlqErr).
					WithField("correlation_id", event.CorrelationID).
					Error("Failed to send event to DLQ")
			}
		}
		
		return fmt.Errorf("handler failed for event type %s: %w", event.Type, err)
	}

	// Успешная обработка
	c.metrics.RecordProcessed(message.Topic, duration)
	
	c.log.WithField("event_type", event.Type).
		WithField("event_id", event.ID).
		WithField("correlation_id", event.CorrelationID).
		WithField("duration_ms", duration.Milliseconds()).
		Info("Event processed successfully")

	return nil
}

// republishEvent отправляет событие обратно в топик для retry
func (c *Consumer) republishEvent(topic string, event *models.Event) error {
	_, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event for retry: %w", err)
	}

	// Используем внутренний producer для отправки
	// (в реальном продакшене лучше использовать отдельный retry топик)
	return c.dlqProducer.publishEvent(topic, *event)
}

// sendToDLQ отправляет событие в Dead Letter Queue
func (c *Consumer) sendToDLQ(originalTopic string, event *models.Event, processingErr error) error {
	dlqTopic := originalTopic + ".dlq"
	
	c.log.WithField("original_topic", originalTopic).
		WithField("dlq_topic", dlqTopic).
		WithField("correlation_id", event.CorrelationID).
		WithField("error", processingErr.Error()).
		Warn("Sending event to DLQ")
	
	// Отправляем в DLQ топик
	return c.dlqProducer.publishEvent(dlqTopic, *event)
}

// StartLagMonitoring запускает мониторинг отставания consumer
func (c *Consumer) StartLagMonitoring(checkInterval time.Duration, lagThreshold int64) {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(checkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				c.checkLag(lagThreshold)
			case <-c.ctx.Done():
				return
			}
		}
	}()

	c.log.WithField("check_interval", checkInterval).
		WithField("lag_threshold", lagThreshold).
		Info("Lag monitoring started")
}

// checkLag проверяет текущее отставание consumer через метрики
func (c *Consumer) checkLag(threshold int64) {
	// Получаем текущую статистику
	stats := c.metrics.GetStats()
	
	// Проверяем критичные условия:
	// 1. Высокий уровень ошибок (больше threshold)
	if stats.TotalErrors > uint64(threshold) {
		c.log.WithField("total_errors", stats.TotalErrors).
			WithField("threshold", threshold).
			Error("ALERT: Total errors exceeded threshold!")
	}
	
	// 2. Высокий error rate (больше 10%)
	if stats.ErrorRate > 10.0 {
		c.log.WithField("error_rate", stats.ErrorRate).
			WithField("total_errors", stats.TotalErrors).
			WithField("total_processed", stats.TotalProcessed).
			Warn("ALERT: High error rate detected in event processing")
	}
	
	// 3. Слишком много событий в DLQ
	if stats.TotalDLQ > 100 {
		c.log.WithField("total_dlq", stats.TotalDLQ).
			Warn("ALERT: High number of events in Dead Letter Queue")
	}
	
	// 4. Слишком много retry попыток
	if stats.TotalRetried > 500 {
		c.log.WithField("total_retried", stats.TotalRetried).
			Warn("ALERT: High number of retry attempts")
	}

	// Логируем текущую статистику для мониторинга
	c.log.WithField("processed", stats.TotalProcessed).
		WithField("errors", stats.TotalErrors).
		WithField("retried", stats.TotalRetried).
		WithField("dlq", stats.TotalDLQ).
		WithField("error_rate", stats.ErrorRate).
		WithField("avg_processing_ms", stats.AvgProcessingTimeMs).
		Debug("Consumer health check completed")
}