package cache

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/config"
	"log"
	"sync"
	"time"
)

var (
	providers   = make(map[string]CacheProvider)
	providersMu sync.RWMutex
	caches      = make(map[string]Cache)
	cachesMu    sync.RWMutex
)

func init() {
	// Built-in memory provider is always available
	RegisterProvider(&memoryProvider{})
}

// RegisterProvider allows adding new cache engines (e.g. Redis)
func RegisterProvider(p CacheProvider) {
	providersMu.Lock()
	defer providersMu.Unlock()
	providers[p.Type()] = p
}

// GetCache returns a cache instance by name, configured via YAML
func GetCache(name string) Cache {
	cachesMu.RLock()
	if c, ok := caches[name]; ok {
		cachesMu.RUnlock()
		return c
	}
	cachesMu.RUnlock()

	cachesMu.Lock()
	defer cachesMu.Unlock()

	// Check again in case it was created during lock wait
	if c, ok := caches[name]; ok {
		return c
	}

	props := config.Get[CacheProperties]()
	if props == nil {
		props = &CacheProperties{Type: "memory", Enabled: true}
	}

	// 1. Determine region configuration
	cacheType := props.Type
	if cacheType == "" {
		cacheType = "memory"
	}

	ttl := 0 * time.Second
	if region, ok := props.Configs[name]; ok {
		if region.Type != "" {
			cacheType = region.Type
		}
		if d, err := time.ParseDuration(region.TTL); err == nil {
			ttl = d
		}
	}

	// 2. Find provider
	providersMu.RLock()
	p, ok := providers[cacheType]
	providersMu.RUnlock()

	if !ok {
		if cacheType == "redis" {
			log.Printf("❌ [Cache] Redis provider not found. Did you import 'github.com/NeftaliAcosta/springo/framework/cache/redis'? ")
			log.Printf("👉 Run: go get github.com/redis/go-redis/v9 && go mod tidy")
		} else {
			log.Printf("⚠️ [Cache] Provider '%s' not found, falling back to 'memory'", cacheType)
		}

		// Fallback to memory
		providersMu.RLock()
		p = providers["memory"]
		providersMu.RUnlock()
	}

	c := p.GetCache(name, ttl)
	caches[name] = c
	return c
}

// Execute wraps a function with cache logic (Cache-Aside pattern)
func Execute[T any](ctx context.Context, region string, key string, fn func() (T, error)) (T, error) {
	c := GetCache(region)

	// 1. Try to get from cache
	if val, ok := c.Get(ctx, key); ok {
		if typedVal, ok := val.(T); ok {
			return typedVal, nil
		}
	}

	// 2. Cache miss: execute function
	result, err := fn()
	if err != nil {
		return result, err
	}

	// 3. Store in cache
	_ = c.Set(ctx, key, result, 0)

	return result, nil
}
