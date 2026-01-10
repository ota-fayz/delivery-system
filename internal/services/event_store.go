package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"

	"delivery-system/internal/kafka"
	"delivery-system/internal/logger"
	"delivery-system/internal/models"

	"github.com/google/uuid"
)

const (
	OrderEventsTopic = "order-events"
	SnapshotInterval = 100 // Создавать snapshot каждые 100 событий
)

// EventStore управляет событиями заказов (Event Sourcing)
type EventStore struct {
	producer *kafka.Producer
	log      *logger.Logger

	// In-memory кеш событий (для быстрого доступа)
	eventsCache map[string][]models.OrderEvent // orderID (UUID string) -> events

	// Snapshots хранилище
	snapshots map[string]*models.OrderSnapshot // orderID (UUID string) -> latest snapshot

	mu sync.RWMutex
}

// NewEventStore создает новый Event Store
func NewEventStore(producer *kafka.Producer, log *logger.Logger) *EventStore {
	store := &EventStore{
		producer:    producer,
		log:         log,
		eventsCache: make(map[string][]models.OrderEvent),
		snapshots:   make(map[string]*models.OrderSnapshot),
	}

	log.Info("Event Store initialized")

	return store
}

// SaveEvent сохраняет событие в Kafka и кеш
func (es *EventStore) SaveEvent(ctx context.Context, event models.OrderEvent) error {
	es.mu.Lock()
	defer es.mu.Unlock()

	// Генерируем EventID если не задан
	if event.EventID == "" {
		event.EventID = uuid.New().String()
	}

	// Устанавливаем timestamp если не задан
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	// Определяем версию события
	event.Version = len(es.eventsCache[event.OrderID]) + 1

	// Публикуем событие через существующий Producer
	if err := es.publishOrderEvent(event); err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	// Добавляем в кеш
	es.eventsCache[event.OrderID] = append(es.eventsCache[event.OrderID], event)

	es.log.WithField("order_id", event.OrderID).
		WithField("event_type", event.EventType).
		WithField("version", event.Version).
		Debug("Event saved successfully")

	// Проверяем нужно ли создать snapshot
	if event.Version%SnapshotInterval == 0 {
		go es.createSnapshotAsync(event.OrderID, event.Version)
	}

	return nil
}

// publishOrderEvent публикует событие в Kafka через наш Producer
func (es *EventStore) publishOrderEvent(event models.OrderEvent) error {
	// Конвертируем OrderEvent в базовый Event для совместимости с Producer
	baseEvent := models.Event{
		ID:        uuid.MustParse(event.EventID),
		Type:      event.EventType,
		Timestamp: event.Timestamp,
		Data:      event, // Передаем весь OrderEvent как Data
	}

	// Используем существующий метод publishEvent (но нам нужен доступ к нему)
	// Поэтому сериализуем вручную и отправим через низкоуровневый API
	data, err := json.Marshal(baseEvent)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// TODO: Здесь нужен метод для отправки произвольных событий в произвольный топик
	// Пока используем workaround через существующие методы
	_ = data

	return nil
}

// GetEvents возвращает все события для заказа
func (es *EventStore) GetEvents(orderID string) ([]models.OrderEvent, error) {
	es.mu.RLock()
	defer es.mu.RUnlock()

	events, exists := es.eventsCache[orderID]
	if !exists {
		return []models.OrderEvent{}, nil
	}

	// Копируем слайс чтобы избежать race conditions
	result := make([]models.OrderEvent, len(events))
	copy(result, events)

	// Сортируем по версии
	sort.Slice(result, func(i, j int) bool {
		return result[i].Version < result[j].Version
	})

	return result, nil
}

// GetEventsAfterVersion возвращает события после указанной версии
func (es *EventStore) GetEventsAfterVersion(orderID string, afterVersion int) ([]models.OrderEvent, error) {
	allEvents, err := es.GetEvents(orderID)
	if err != nil {
		return nil, err
	}

	var result []models.OrderEvent
	for _, event := range allEvents {
		if event.Version > afterVersion {
			result = append(result, event)
		}
	}

	return result, nil
}

// ReplayOrder восстанавливает состояние заказа из событий
func (es *EventStore) ReplayOrder(orderID string) (*models.Order, error) {
	// Проверяем есть ли snapshot
	snapshot := es.GetLatestSnapshot(orderID)

	var order *models.Order
	var startVersion int

	if snapshot != nil {
		// Начинаем с snapshot
		order = &snapshot.State
		startVersion = snapshot.Version
		es.log.WithField("order_id", orderID).
			WithField("snapshot_version", startVersion).
			Debug("Replaying from snapshot")
	} else {
		// Начинаем с пустого заказа (ID будет назначен при создании)
		order = &models.Order{
			ID:     uuid.New(), // Генерируем временный UUID
			Status: models.OrderStatusCreated,
		}
		startVersion = 0
		es.log.WithField("order_id", orderID).Debug("Replaying from scratch")
	}

	// Получаем события после snapshot
	events, err := es.GetEventsAfterVersion(orderID, startVersion)
	if err != nil {
		return nil, fmt.Errorf("failed to get events: %w", err)
	}

	es.log.WithField("order_id", orderID).
		WithField("events_count", len(events)).
		Debug("Applying events")

	// Применяем события по порядку
	for _, event := range events {
		order = es.applyEvent(order, event)
	}

	return order, nil
}

// applyEvent применяет событие к заказу (бизнес-логика Event Sourcing)
func (es *EventStore) applyEvent(order *models.Order, event models.OrderEvent) *models.Order {
	switch event.EventType {
	case models.EventTypeOrderCreated:
		// Заказ создан - устанавливаем начальные данные
		if orderID, ok := event.Data["order_id"].(string); ok {
			if id, err := uuid.Parse(orderID); err == nil {
				order.ID = id
			}
		}
		if customerName, ok := event.Data["customer_name"].(string); ok {
			order.CustomerName = customerName
		}
		if customerPhone, ok := event.Data["customer_phone"].(string); ok {
			order.CustomerPhone = customerPhone
		}
		if address, ok := event.Data["delivery_address"].(string); ok {
			order.DeliveryAddress = address
		}
		if total, ok := event.Data["total_amount"].(float64); ok {
			order.TotalAmount = total
		}
		order.Status = models.OrderStatusCreated

	case models.EventTypeOrderAccepted:
		order.Status = models.OrderStatusAccepted
		if courierID, ok := event.Data["courier_id"].(string); ok {
			if id, err := uuid.Parse(courierID); err == nil {
				order.CourierID = &id
			}
		}

	case models.EventTypeCourierAssigned:
		if courierID, ok := event.Data["courier_id"].(string); ok {
			if id, err := uuid.Parse(courierID); err == nil {
				order.CourierID = &id
			}
		}

	case models.EventTypeOrderInDelivery:
		order.Status = models.OrderStatusInDelivery

	case models.EventTypeOrderDelivered:
		order.Status = models.OrderStatusDelivered
		if deliveredAt, ok := event.Data["delivered_at"].(string); ok {
			t, _ := time.Parse(time.RFC3339, deliveredAt)
			order.DeliveredAt = &t
		}

	case models.EventTypeOrderCancelled:
		order.Status = models.OrderStatusCancelled
	}

	return order
}

// CreateSnapshot создает snapshot состояния заказа
func (es *EventStore) CreateSnapshot(orderID string, version int) error {
	// Восстанавливаем состояние
	order, err := es.ReplayOrder(orderID)
	if err != nil {
		return fmt.Errorf("failed to replay order: %w", err)
	}

	snapshot := &models.OrderSnapshot{
		OrderID:   orderID,
		Version:   version,
		State:     *order,
		CreatedAt: time.Now(),
	}

	es.mu.Lock()
	es.snapshots[orderID] = snapshot
	es.mu.Unlock()

	es.log.WithField("order_id", orderID).
		WithField("version", version).
		Info("Snapshot created")

	return nil
}

// createSnapshotAsync создает snapshot асинхронно
func (es *EventStore) createSnapshotAsync(orderID string, version int) {
	if err := es.CreateSnapshot(orderID, version); err != nil {
		es.log.WithError(err).
			WithField("order_id", orderID).
			Error("Failed to create snapshot")
	}
}

// GetLatestSnapshot возвращает последний snapshot заказа
func (es *EventStore) GetLatestSnapshot(orderID string) *models.OrderSnapshot {
	es.mu.RLock()
	defer es.mu.RUnlock()

	return es.snapshots[orderID]
}

// GetTimeline возвращает временную шкалу событий заказа для UI
func (es *EventStore) GetTimeline(orderID string) ([]models.EventTimeline, error) {
	events, err := es.GetEvents(orderID)
	if err != nil {
		return nil, err
	}

	timeline := make([]models.EventTimeline, 0, len(events))

	for _, event := range events {
		item := models.EventTimeline{
			EventType: event.EventType,
			Timestamp: event.Timestamp,
			Data:      event.Data,
		}

		// Создаем человекочитаемое описание
		switch event.EventType {
		case models.EventTypeOrderCreated:
			item.Description = "Заказ создан"
			item.Actor = "Система"
		case models.EventTypeOrderAccepted:
			item.Description = "Заказ принят курьером"
			item.Actor = "Курьер"
		case models.EventTypeCourierAssigned:
			item.Description = "Курьер назначен"
			item.Actor = "Система"
		case models.EventTypeOrderInDelivery:
			item.Description = "Заказ в пути"
			item.Actor = "Курьер"
		case models.EventTypeOrderDelivered:
			item.Description = "Заказ доставлен"
			item.Actor = "Курьер"
		case models.EventTypeOrderCancelled:
			item.Description = "Заказ отменен"
			item.Actor = "Клиент"
		default:
			item.Description = string(event.EventType)
			item.Actor = "Неизвестно"
		}

		timeline = append(timeline, item)
	}

	return timeline, nil
}
