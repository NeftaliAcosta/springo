package cache

import (
	"context"
	"sync"
	"time"
)

type memoryProvider struct{}

func (p *memoryProvider) Type() string { return "memory" }

func (p *memoryProvider) GetCache(name string, ttl time.Duration) Cache {
	return &memoryCache{
		name: name,
		ttl:  ttl,
		data: make(map[string]cacheItem),
	}
}

type cacheItem struct {
	value      any
	expiration int64
}

type memoryCache struct {
	name string
	ttl  time.Duration
	data map[string]cacheItem
	mu   sync.RWMutex
}

func (c *memoryCache) Name() string { return c.name }

func (c *memoryCache) Get(ctx context.Context, key string) (any, bool) {
	c.mu.RLock()
	item, ok := c.data[key]
	c.mu.RUnlock()

	if !ok {
		return nil, false
	}

	if item.expiration > 0 && time.Now().UnixNano() > item.expiration {
		c.Evict(ctx, key)
		return nil, false
	}

	return item.value, true
}

func (c *memoryCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	var expiration int64
	if ttl == 0 {
		ttl = c.ttl
	}

	if ttl > 0 {
		expiration = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	c.data[key] = cacheItem{value: value, expiration: expiration}
	c.mu.Unlock()
	return nil
}

func (c *memoryCache) Evict(ctx context.Context, key string) error {
	c.mu.Lock()
	delete(c.data, key)
	c.mu.Unlock()
	return nil
}

func (c *memoryCache) Clear(ctx context.Context) error {
	c.mu.Lock()
	c.data = make(map[string]cacheItem)
	c.mu.Unlock()
	return nil
}


func (c *memoryCache) Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	var expiration int64
	if ttl == 0 {
		ttl = c.ttl
	}

	if ttl > 0 {
		expiration = time.Now().Add(ttl).UnixNano()
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	item, ok := c.data[key]
	var current int64
	if ok {
		if item.expiration > 0 && time.Now().UnixNano() > item.expiration {
			current = 0 // Expired
		} else {
			switch v := item.value.(type) {
			case int:
				current = int64(v)
			case int64:
				current = v
			case float64:
				current = int64(v)
			default:
				current = 0
			}
		}
	}
	
	current += delta
	c.data[key] = cacheItem{value: current, expiration: expiration}
	return current, nil
}
