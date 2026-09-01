package redis

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/NeftaliAcosta/springo/framework/cache"
	"github.com/NeftaliAcosta/springo/framework/config"
	goredis "github.com/redis/go-redis/v9"
)

type RedisProperties struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Password string `yaml:"password"`
	DB       int    `yaml:"db"`
}

func init() {
	config.RegisterProperties("redis", &RedisProperties{
		Host: "localhost",
		Port: 6379,
		DB:   0,
	})
	Enable()
}

var client *goredis.Client

func getClient() *goredis.Client {
	if client != nil {
		return client
	}
	props := config.Get[RedisProperties]()
	if props == nil {
		props = &RedisProperties{Host: "localhost", Port: 6379, DB: 0}
	}
	client = goredis.NewClient(&goredis.Options{
		Addr:     fmt.Sprintf("%s:%d", props.Host, props.Port),
		Password: props.Password,
		DB:       props.DB,
	})
	
	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("⚠️ [Cache] Failed to connect to Redis: %v", err)
	} else {
		log.Printf("✅ [Cache] Connected to Redis at %s:%d", props.Host, props.Port)
	}
	return client
}

type redisProvider struct{}

func (p *redisProvider) Type() string { return "redis" }

func (p *redisProvider) GetCache(name string, ttl time.Duration) cache.Cache {
	return &redisCache{
		name:   name,
		ttl:    ttl,
		client: getClient(),
	}
}

type redisCache struct {
	name   string
	ttl    time.Duration
	client *goredis.Client
}

func (c *redisCache) Name() string { return c.name }

func (c *redisCache) key(k string) string {
	return fmt.Sprintf("%s:%s", c.name, k)
}

func (c *redisCache) Get(ctx context.Context, key string) (any, bool) {
	val, err := c.client.Get(ctx, c.key(key)).Result()
	if err != nil {
		return nil, false
	}
	return val, true
}

func (c *redisCache) Set(ctx context.Context, key string, value any, ttl time.Duration) error {
	t := ttl
	if t == 0 {
		t = c.ttl
	}
	return c.client.Set(ctx, c.key(key), value, t).Err()
}

func (c *redisCache) Increment(ctx context.Context, key string, delta int64, ttl time.Duration) (int64, error) {
	t := ttl
	if t == 0 {
		t = c.ttl
	}
	
	k := c.key(key)
	
	pipe := c.client.Pipeline()
	incr := pipe.IncrBy(ctx, k, delta)
	if t > 0 {
		pipe.Expire(ctx, k, t)
	}
	
	_, err := pipe.Exec(ctx)
	if err != nil {
		return 0, err
	}
	return incr.Val(), nil
}

func (c *redisCache) Evict(ctx context.Context, key string) error {
	return c.client.Del(ctx, c.key(key)).Err()
}

func (c *redisCache) Clear(ctx context.Context) error {
	iter := c.client.Scan(ctx, 0, c.name+":*", 0).Iterator()
	var errs []error
	for iter.Next(ctx) {
		err := c.client.Del(ctx, iter.Val()).Err()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if err := iter.Err(); err != nil {
		return err
	}
	if len(errs) > 0 {
		return fmt.Errorf("failed to clear all keys")
	}
	return nil
}

// Enable registers the redis provider
func Enable() {
	cache.RegisterProvider(&redisProvider{})
}
