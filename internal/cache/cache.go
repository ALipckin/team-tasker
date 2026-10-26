package cache

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

// ErrMiss — ключа нет в кэше.
var ErrMiss = errors.New("cache miss")

// Cache — интерфейс подключения к кэшу.
// Структура неэкспортируемая — создать объект можно только через New.
type Cache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string, ttl time.Duration) error
	Incr(ctx context.Context, key string) (int64, error)
	Close() error
}

// redisCache — обёртка над подключением к Redis.
// Все запросы идут через circuit breaker: если Redis недоступен, мы быстро
// отдаём ошибку, и сервисы работают напрямую с БД, не дожидаясь таймаутов.
type redisCache struct {
	client  *redis.Client
	breaker *breaker
}

// New создаёт подключение к кэшу. Недоступность Redis при старте не фатальна.
func New(addr, password string, db int) Cache {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
		// Кэш — необязательная зависимость: не ретраим и быстро сдаёмся,
		// чтобы недоступный Redis не замедлял ответы из БД.
		MaxRetries:   -1,
		DialTimeout:  300 * time.Millisecond,
		ReadTimeout:  300 * time.Millisecond,
		WriteTimeout: 300 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		slog.Warn("кэш недоступен, работаем без него до восстановления", "addr", addr, "err", err)
	} else {
		slog.Info("подключились к кэшу", "addr", addr)
	}

	return &redisCache{
		client:  client,
		breaker: newBreaker(5, 10*time.Second),
	}
}

// Get получает значение по ключу. Если ключа нет — ErrMiss.
func (c *redisCache) Get(ctx context.Context, key string) (string, error) {
	var value string

	err := c.breaker.do(func() error {
		v, err := c.client.Get(ctx, key).Result()
		if errors.Is(err, redis.Nil) {
			return nil
		}
		value = v
		return err
	})
	if err != nil {
		return "", fmt.Errorf("cache get: %w", err)
	}

	if value == "" {
		return "", ErrMiss
	}

	return value, nil
}

// Set устанавливает значение по ключу с TTL.
func (c *redisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	return c.breaker.do(func() error {
		return c.client.Set(ctx, key, value, ttl).Err()
	})
}

// Incr атомарно увеличивает счётчик.
func (c *redisCache) Incr(ctx context.Context, key string) (int64, error) {
	var n int64

	err := c.breaker.do(func() error {
		var err error
		n, err = c.client.Incr(ctx, key).Result()
		return err
	})

	return n, err
}

// Close закрывает подключение к кэшу.
func (c *redisCache) Close() error {
	slog.Info("подключение к кэшу закрыто")
	return c.client.Close()
}
