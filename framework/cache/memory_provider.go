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

// DefaultMaxMemoryCacheKeys defines the maximum number of items per in-memory cache instance.
const DefaultMaxMemoryCacheKeys = 10000

func (c *memoryCache) Name() string { return c.name }

func (c *memoryCache) Get(ctx context.Context, key string) (any, bool) {
	c.mu.RLock()
	item, ok := c.data[key]
	if !ok {
		c.mu.RUnlock()
		return nil, false
	}

	if item.expiration > 0 && time.Now().UnixNano() > item.expiration {
		c.mu.RUnlock()
		c.mu.Lock()
		if curItem, stillOk := c.data[key]; stillOk && curItem.expiration > 0 && time.Now().UnixNano() > curItem.expiration {
			delete(c.data, key)
		}
		c.mu.Unlock()
		return nil, false
	}

	c.mu.RUnlock()
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
	defer c.mu.Unlock()

	// If capacity exceeded and key is new, evict expired items
	if _, exists := c.data[key]; !exists && len(c.data) >= DefaultMaxMemoryCacheKeys {
		c.evictExpiredItems()
		// If still at capacity, drop an entry
		if len(c.data) >= DefaultMaxMemoryCacheKeys {
			for k := range c.data {
				delete(c.data, k)
				break
			}
		}
	}

	c.data[key] = cacheItem{value: value, expiration: expiration}
	return nil
}

func (c *memoryCache) evictExpiredItems() {
	now := time.Now().UnixNano()
	for k, item := range c.data {
		if item.expiration > 0 && now > item.expiration {
			delete(c.data, k)
		}
	}
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
