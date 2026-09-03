# ⚡ Step-by-Step Guide: Cache Abstraction & Redis Integration

This tutorial explains how to configure and use in-memory and Redis caching in SprinGo microservices.

---

## 1. Overview

SprinGo provides a unified caching subsystem:
- **Multi-Driver Architecture**: Seamlessly switch between in-memory cache and Redis via `application.yaml`.
- **TTL Expiration & Eviction**: Configurable item time-to-live and sliding expirations.
- **Serialization Safety**: Type-safe storage and retrieval with JSON / binary serialization.
- **Actuator Health Integration**: Automatic health monitoring probe reporting Redis connection health.

---

## 2. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  cache:
    type: redis # Options: memory, redis
    redis:
      host: ${REDIS_HOST:localhost}
      port: ${REDIS_PORT:6379}
      password: ${REDIS_PASSWORD:}
      db: 0
      default-ttl: 15m
```

---

## 3. Using the Cache in Services

**Suggested File Path**: `internal/application/service/product_service.go`
```go
package service

import (
    "context"
    "fmt"
    "time"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/port/output"
    "github.com/NeftaliAcosta/springo/framework/cache"
)

type ProductService struct {
    cacheManager cache.Cache              `spring:"cacheManager"`
    repo         output.ProductRepository `spring:"productRepository"`
}

func (s *ProductService) GetProduct(ctx context.Context, id uint) (*model.Product, error) {
    cacheKey := fmt.Sprintf("products:%d", id)

    // 1. Try fetching from cache
    var product model.Product
    found, err := s.cacheManager.Get(ctx, cacheKey, &product)
    if err == nil && found {
        return &product, nil
    }

    // 2. Fetch from database on cache miss
    dbProduct, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return nil, err
    }

    // 3. Populate cache with 10-minute TTL
    _ = s.cacheManager.Set(ctx, cacheKey, dbProduct, 10*time.Minute)

    return dbProduct, nil
}

func (s *ProductService) UpdateProduct(ctx context.Context, p *model.Product) error {
    if err := s.repo.Update(ctx, p); err != nil {
        return err
    }

    // Evict stale cache entry
    cacheKey := fmt.Sprintf("products:%d", p.ID)
    return s.cacheManager.Delete(ctx, cacheKey)
}
```
