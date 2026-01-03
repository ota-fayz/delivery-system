package services

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"delivery-system/internal/config"
	"delivery-system/internal/logger"
	"delivery-system/internal/redis"
)

// CacheService управляет кешированием данных
type CacheService struct {
	redis     *redis.Client
	config    *config.CacheConfig
	logger    *logger.Logger
	hits      atomic.Uint64 // Количество попаданий в кеш
	misses    atomic.Uint64 // Количество промахов
	evictions atomic.Uint64 // Количество инвалидаций
}

// CacheMetrics представляет метрики кеширования
type CacheMetrics struct {
	Hits      uint64  `json:"hits"`
	Misses    uint64  `json:"misses"`
	Evictions uint64  `json:"evictions"`
	TotalReqs uint64  `json:"total_requests"`
	HitRate   float64 `json:"hit_rate"`
	CacheSize int64   `json:"cache_size"`
}

// NewCacheService создает новый сервис кеширования
func NewCacheService(redis *redis.Client, cfg *config.CacheConfig, log *logger.Logger) *CacheService {
	return &CacheService{
		redis:  redis,
		config: cfg,
		logger: log,
	}
}

// Get получает данные из кеша и десериализует в target
func (s *CacheService) Get(ctx context.Context, key string, target interface{}) (bool, error) {
	if !s.config.Enabled {
		s.misses.Add(1)
		return false, nil
	}

	err := s.redis.Get(ctx, key, target)
	if err != nil {
		// Любая ошибка "not found" считается miss (данных нет в кеше)
		if strings.Contains(err.Error(), "not found") {
			s.misses.Add(1)
			return false, nil
		}
		// Другие ошибки (сеть, парсинг) - логируем и возвращаем
		s.logger.Error("Failed to get from cache", "key", key, "error", err)
		return false, err
	}

	s.hits.Add(1)
	return true, nil
}

// Set сохраняет данные в кеш с TTL
func (s *CacheService) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	if !s.config.Enabled {
		return nil
	}

	err := s.redis.Set(ctx, key, value, ttl)
	if err != nil {
		s.logger.Error("Failed to set cache", "key", key, "error", err)
		return err
	}

	return nil
}

// Delete удаляет ключ из кеша (инвалидация)
func (s *CacheService) Delete(ctx context.Context, keys ...string) error {
	if !s.config.Enabled {
		return nil
	}

	client := s.redis.GetClient()
	pipe := client.Pipeline()

	for _, key := range keys {
		pipe.Del(ctx, key)
	}

	_, err := pipe.Exec(ctx)
	if err != nil {
		s.logger.Error("Failed to delete from cache", "keys", keys, "error", err)
		return err
	}

	s.evictions.Add(uint64(len(keys)))
	return nil
}

// GetMetrics возвращает метрики кеширования
func (s *CacheService) GetMetrics(ctx context.Context) (*CacheMetrics, error) {
	hits := s.hits.Load()
	misses := s.misses.Load()
	evictions := s.evictions.Load()
	totalReqs := hits + misses

	var hitRate float64
	if totalReqs > 0 {
		hitRate = float64(hits) / float64(totalReqs) * 100
	}

	// Получаем размер кеша (количество ключей)
	client := s.redis.GetClient()
	cacheSize, err := client.DBSize(ctx).Result()
	if err != nil {
		s.logger.Error("Failed to get cache size", "error", err)
		cacheSize = 0
	}

	return &CacheMetrics{
		Hits:      hits,
		Misses:    misses,
		Evictions: evictions,
		TotalReqs: totalReqs,
		HitRate:   hitRate,
		CacheSize: cacheSize,
	}, nil
}

// GetDefaultTTL возвращает TTL по умолчанию
func (s *CacheService) GetDefaultTTL() time.Duration {
	return time.Duration(s.config.DefaultTTL) * time.Second
}

// GetHotDataTTL возвращает TTL для горячих данных
func (s *CacheService) GetHotDataTTL() time.Duration {
	return time.Duration(s.config.HotDataTTL) * time.Second
}

// BuildKey создает ключ для кеша с префиксом
func BuildKey(prefix string, id string) string {
	return fmt.Sprintf("%s:%s", prefix, id)
}

// BuildListKey создает ключ для списка с фильтрами
func BuildListKey(prefix string, filters ...string) string {
	key := prefix + ":list"
	for _, f := range filters {
		key += ":" + f
	}
	return key
}

// WarmupCache прогревает кеш популярными данными при старте приложения
// Принимает функции для загрузки данных из БД
func (s *CacheService) WarmupCache(ctx context.Context, warmupFuncs map[string]func() (interface{}, error)) {
	if !s.config.Enabled {
		s.logger.Info("Cache warming skipped (cache disabled)")
		return
	}

	s.logger.Info("Starting cache warming...")
	successCount := 0

	for key, fetchFunc := range warmupFuncs {
		data, err := fetchFunc()
		if err != nil {
			s.logger.Error("Failed to fetch data for cache warming", "key", key, "error", err)
			continue
		}

		// Используем hot data TTL для прогретых данных
		if err := s.Set(ctx, key, data, s.GetHotDataTTL()); err != nil {
			s.logger.Error("Failed to warm cache", "key", key, "error", err)
			continue
		}

		successCount++
	}

	s.logger.Info("Cache warming completed", "success", successCount, "total", len(warmupFuncs))
}
