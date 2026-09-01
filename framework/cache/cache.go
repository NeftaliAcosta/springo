package cache

import (
	"context"
	"time"
)

// Cache is the interface for a named cache region
type Cache interface {
	Name() string
	Get(ctx context.Context, key string) (any, bool)
	Set(ctx context.Context, key string, value any, ttl time.Duration) error
	Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error)
	Evict(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// CacheProvider is the engine responsible for creating and managing Cache instances
type CacheProvider interface {
	Type() string
	GetCache(name string, ttl time.Duration) Cache
}
