package cache

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

const menuTTL = 5 * time.Minute

type MenuRedis struct {
	c   redis.UniversalClient
	ttl time.Duration
}

func NewMenuRedis(addr string) (*MenuRedis, error) {
	if addr == "" {
		return nil, errors.New("empty redis addr")
	}
	c := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := c.Ping(ctx).Err(); err != nil {
		_ = c.Close()
		return nil, err
	}
	return &MenuRedis{c: c, ttl: menuTTL}, nil
}

func (m *MenuRedis) Close() error {
	if m == nil || m.c == nil {
		return nil
	}
	return m.c.Close()
}

func (m *MenuRedis) Get(ctx context.Context, key string) ([]byte, bool) {
	if m == nil {
		return nil, false
	}
	b, err := m.c.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, false
	}
	if err != nil {
		slog.Warn("redis menu get", "key", key, "err", err)
		return nil, false
	}
	return b, true
}

func (m *MenuRedis) Set(ctx context.Context, key string, body []byte) {
	if m == nil {
		return
	}
	if err := m.c.Set(ctx, key, body, m.ttl).Err(); err != nil {
		slog.Warn("redis menu set", "key", key, "err", err)
	}
}
