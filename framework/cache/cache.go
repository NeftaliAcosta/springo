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
	Evict(ctx context.Context, key string) error
	Clear(ctx context.Context) error
}

// CacheProvider is the engine responsible for creating and managing Cache instances
type CacheProvider interface {
	Type() string
	GetCache(name string, ttl time.Duration) Cache
}
