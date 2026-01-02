package services

import (
	"delivery-system/internal/config"
	"delivery-system/internal/logger"
	"fmt"
	"math"
	"math/rand"
)

type DeliveryPricingService struct {
	config *config.DeliveryPricingConfig
	log    *logger.Logger
}

func NewDeliveryPricingService(cfg *config.DeliveryPricingConfig, log *logger.Logger) *DeliveryPricingService {
	return &DeliveryPricingService{
		config: cfg,
		log:    log,
	}
}

func (s *DeliveryPricingService) CalculateDeliveryCost(pickupAddress, deliveryAddress string) (float64, error) {
	if pickupAddress == "" || deliveryAddress == "" {
		return 0, fmt.Errorf("addresses cannot be empty")
	}

	distance, err := s.calculateDistance(pickupAddress, deliveryAddress)

	if err != nil {
		return 0, err
	}

	cost := s.config.BasePrice + (distance * s.config.PricePerKm)

	if cost < s.config.MinPrice {
		cost = s.config.MinPrice
	}

	if cost > s.config.MaxPrice {
		cost = s.config.MaxPrice
	}

	return cost, nil
}

func (s *DeliveryPricingService) calculateDistance(addr1, addr2 string) (float64, error) {
	// TODO (из README - Задача #3):
	// 1. Интегрировать с Yandex Maps API для геокодирования адресов
	// 2. Добавить Redis кеширование результатов геокодирования
	// 3. Добавить возможность ручного override стоимости доставки

	// Сейчас используется упрощенная версия с случайным расстоянием (1-20 км)
	distance := 1.0 + rand.Float64()*19.0
	distance = math.Round(distance*100) / 100

	return distance, nil
}
