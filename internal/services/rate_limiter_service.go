package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"delivery-system/internal/config"
	"delivery-system/internal/logger"
	"delivery-system/internal/redis"
)

// Lua скрипт для атомарной проверки и инкремента счетчика
const rateLimitLuaScript = `
local key = KEYS[1]
local limit = tonumber(ARGV[1])
local ttl = tonumber(ARGV[2])

local current = redis.call('GET', key)
if not current then
    current = 0
else
    current = tonumber(current)
end

current = current + 1

if current > limit then
    return {0, current, limit}
end

redis.call('SET', key, current, 'EX', ttl)
return {1, current, limit}
`

// RateLimiterService управляет rate limiting с использованием Redis
type RateLimiterService struct {
	redis  *redis.Client
	config *config.RateLimitConfig
	log    *logger.Logger
}

// RateLimitResult содержит результат проверки rate limit
type RateLimitResult struct {
	Allowed     bool
	Remaining   int
	Limit       int
	ResetAt     time.Time
	BannedUntil time.Time
	RetryAfter  int
}

func NewRateLimiterService(redis *redis.Client, cfg *config.RateLimitConfig, log *logger.Logger) *RateLimiterService {
	return &RateLimiterService{
		redis:  redis,
		config: cfg,
		log:    log,
	}
}

func (s *RateLimiterService) CheckLimit(ctx context.Context, ip string, isVIP bool) (*RateLimitResult, error) {
	if !s.config.Enabled {
		return &RateLimitResult{
			Allowed:   true,
			Remaining: math.MaxInt,
			Limit:     math.MaxInt,
		}, nil
	}

	client := s.redis.GetClient()

	limit := s.config.DefaultRPM
	if isVIP {
		limit = s.config.VIPRPM
	}

	banKey := fmt.Sprintf("rate_limit:ban:%s", ip)
	banned, err := client.Get(ctx, banKey).Result()
	if err == nil && banned != "" {
		// Пользователь забанен
		ttl, _ := client.TTL(ctx, banKey).Result()
		return &RateLimitResult{
			Allowed:     false,
			Remaining:   0,
			Limit:       limit,
			BannedUntil: time.Now().Add(ttl),
			RetryAfter:  int(ttl.Seconds()),
		}, nil
	}

	key := fmt.Sprintf("rate_limit:ip:%s", ip)

	result, err := client.Eval(ctx, rateLimitLuaScript, []string{key}, limit, 60).Result()
	if err != nil {
		s.log.Error("Ошибка выполнения Lua скрипта", "ip", ip, "error", err)
		// При ошибке пропускаем запрос (fail-open)
		return &RateLimitResult{
			Allowed:   true,
			Remaining: limit,
			Limit:     limit,
		}, nil
	}

	resultSlice, ok := result.([]interface{})
	if !ok || len(resultSlice) != 3 {
		s.log.Error("Некорректный результат Lua скрипта", "ip", ip, "result", result)
		return &RateLimitResult{
			Allowed:   true,
			Remaining: limit,
			Limit:     limit,
		}, nil
	}

	allowed := resultSlice[0].(int64) == 1
	currentCount := int(resultSlice[1].(int64))

	if !allowed {
		client.Set(ctx, banKey, "1", time.Duration(s.config.BanDuration)*time.Second)

		s.log.Warn("Пользователь превысил rate limit и забанен",
			"ip", ip,
			"count", currentCount,
			"limit", limit,
			"ban_duration", s.config.BanDuration)

		return &RateLimitResult{
			Allowed:     false,
			Remaining:   0,
			Limit:       limit,
			BannedUntil: time.Now().Add(time.Duration(s.config.BanDuration) * time.Second),
			RetryAfter:  s.config.BanDuration,
		}, nil
	}

	ttl, _ := client.TTL(ctx, key).Result()
	resetAt := time.Now().Add(ttl)

	return &RateLimitResult{
		Allowed:   true,
		Remaining: limit - currentCount,
		Limit:     limit,
		ResetAt:   resetAt,
	}, nil
}

func (s *RateLimiterService) ResetLimit(ctx context.Context, ip string) error {
	client := s.redis.GetClient()

	key := fmt.Sprintf("rate_limit:ip:%s", ip)
	banKey := fmt.Sprintf("rate_limit:ban:%s", ip)

	pipe := client.Pipeline()
	pipe.Del(ctx, key)
	pipe.Del(ctx, banKey)
	_, err := pipe.Exec(ctx)

	if err != nil {
		s.log.Error("Ошибка сброса rate limit в Redis", "ip", ip, "error", err)
		return err
	}

	s.log.Info("Rate limit сброшен", "ip", ip)
	return nil
}

// GetStatus возвращает текущий статус rate limit БЕЗ изменения счетчика
func (s *RateLimiterService) GetStatus(ctx context.Context, ip string, isVIP bool) (*RateLimitResult, error) {
	// Если rate limiting выключен
	if !s.config.Enabled {
		return &RateLimitResult{
			Allowed:   true,
			Remaining: math.MaxInt,
			Limit:     math.MaxInt,
		}, nil
	}

	client := s.redis.GetClient()

	// Определяем лимит
	limit := s.config.DefaultRPM
	if isVIP {
		limit = s.config.VIPRPM
	}

	// Проверяем бан
	banKey := fmt.Sprintf("rate_limit:ban:%s", ip)
	banned, err := client.Get(ctx, banKey).Result()
	if err == nil && banned != "" {
		ttl, _ := client.TTL(ctx, banKey).Result()
		return &RateLimitResult{
			Allowed:     false,
			Remaining:   0,
			Limit:       limit,
			BannedUntil: time.Now().Add(ttl),
			RetryAfter:  int(ttl.Seconds()),
		}, nil
	}

	// Ключ для счетчика
	key := fmt.Sprintf("rate_limit:ip:%s", ip)

	// ЧИТАЕМ счетчик БЕЗ изменения (не используем Lua скрипт!)
	count, err := client.Get(ctx, key).Int()
	if err != nil {
		// Ключа нет или ошибка - значит запросов еще не было
		count = 0
	}

	// Вычисляем оставшиеся запросы
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}

	// Получаем TTL ключа
	ttl, _ := client.TTL(ctx, key).Result()
	resetAt := time.Now().Add(ttl)

	// Если ключа нет (TTL < 0), reset_at не имеет смысла
	if ttl < 0 {
		resetAt = time.Time{}
	}

	return &RateLimitResult{
		Allowed:   count < limit,
		Remaining: remaining,
		Limit:     limit,
		ResetAt:   resetAt,
	}, nil
}
