package cache

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestMemoryCache_GetSetEvict(t *testing.T) {
	provider := &memoryProvider{}
	c := provider.GetCache("test-cache", 100*time.Millisecond)

	ctx := context.Background()
	err := c.Set(ctx, "key1", "val1", 0)
	assert.NoError(t, err)

	val, ok := c.Get(ctx, "key1")
	assert.True(t, ok)
	assert.Equal(t, "val1", val)

	err = c.Evict(ctx, "key1")
	assert.NoError(t, err)

	_, ok = c.Get(ctx, "key1")
	assert.False(t, ok)
}

func TestMemoryCache_Expiration(t *testing.T) {
	provider := &memoryProvider{}
	c := provider.GetCache("test-cache-exp", 50*time.Millisecond)

	ctx := context.Background()
	err := c.Set(ctx, "tempKey", "tempVal", 50*time.Millisecond)
	assert.NoError(t, err)

	val, ok := c.Get(ctx, "tempKey")
	assert.True(t, ok)
	assert.Equal(t, "tempVal", val)

	time.Sleep(60 * time.Millisecond)

	_, ok = c.Get(ctx, "tempKey")
	assert.False(t, ok)
}

func TestMemoryCache_Increment(t *testing.T) {
	provider := &memoryProvider{}
	c := provider.GetCache("test-cache-inc", 0)

	ctx := context.Background()
	n, err := c.Increment(ctx, "counter", 5, 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(5), n)

	n, err = c.Increment(ctx, "counter", 3, 0)
	assert.NoError(t, err)
	assert.Equal(t, int64(8), n)
}
