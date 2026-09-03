# 🗄️ Step-by-Step Guide: Multiple DataSources & Connection Pooling

This tutorial explains how to configure primary, secondary, and read-replica database connections with custom pool
tuning in SprinGo.

---

## 1. Overview

SprinGo natively supports multi-database topologies:
- **Primary DataSource (`spring.datasource`)**: The default GORM connection used for migrations, IoC singletons,
  and transaction propagation.
- **Additional Named DataSources (`spring.additional-datasources`)**: Supplementary connections for read replicas,
  analytics warehouses, or multi-tenant databases.
- **Automated Health Monitoring (`health-check: true`)**: Automatically discovers and monitors connection liveness in
  the Actuator dashboard.
- **Smart Connection Pool Tuning**: Dialect-aware defaults for max open connections, idle connections, and lifetime.

---

## 2. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  # Primary Database (Read/Write)
  datasource:
    driver: mysql
    url: "${DB_DSN:root:secret@tcp(127.0.0.1:3306)/main_db?charset=utf8mb4&parseTime=True&loc=Local}"
    auto-migrate: true
    health-check: true
    migration-table: springo_migrations
    migration-lock-timeout: 5m

  # Secondary Named DataSources
  additional-datasources:
    # Read-Only Replica
    readonly:
      driver: mysql
      url: "${DB_READONLY_DSN:root:secret@tcp(127.0.0.1:3307)/main_db?charset=utf8mb4&parseTime=True&loc=Local}"
      health-check: true

    # Analytics Warehouse (PostgreSQL)
    analytics:
      driver: postgres
      url: "${ANALYTICS_DB_URL:postgres://user:pass@analytics-db:5432/warehouse?sslmode=disable}"
      health-check: true
```

---

## 3. Initializing and Registering Named DataSources

**Suggested File Path**: `internal/infrastructure/config/additional_datasources_lifecycle.go`
```go
package config

import (
    "context"
    "fmt"
    "log/slog"

    frameworkConfig "github.com/NeftaliAcosta/springo/framework/config"
    "github.com/NeftaliAcosta/springo/framework/database"
    "github.com/NeftaliAcosta/springo/framework/ioc"
    "github.com/NeftaliAcosta/springo/framework/lifecycle"
)

func init() {
    lifecycle.RegisterInitializer("database.additional_datasources", 15, func(ctx context.Context) error {
        additional := frameworkConfig.Get[database.AdditionalDataSources]()
        if additional == nil {
            return nil
        }

        for name, props := range *additional {
            propsCopy := props
            db, err := database.Connect(&propsCopy)
            if err != nil {
                return fmt.Errorf("failed to connect additional datasource '%s': %w", name, err)
            }

            // Register bean in IoC container (e.g. 'readonlyDB', 'analyticsDB')
            beanName := name + "DB"
            ioc.RegisterBean(beanName, db)
            slog.Info("Registered additional datasource", "name", beanName, "driver", props.Driver)
        }

        return nil
    })
}
```

---

## 4. Injecting and Using Named DataSources

**Suggested File Path**: `internal/infrastructure/output/persistence/product_repository_adapter.go`
```go
package persistence

import (
    "context"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
    "gorm.io/gorm"
)

type ProductRepositoryAdapter struct {
    primaryDB  *gorm.DB `spring:"db"`         // Injected primary database
    readonlyDB *gorm.DB `spring:"readonlyDB"` // Injected read replica
}

func (r *ProductRepositoryAdapter) FindByID(ctx context.Context, id uint) (*model.Product, error) {
    var product model.Product
    // Query read replica to offload the primary database
    err := r.readonlyDB.WithContext(ctx).First(&product, id).Error
    return &product, err
}

func (r *ProductRepositoryAdapter) Create(ctx context.Context, p *model.Product) error {
    // Write operations always target the primary database
    return r.primaryDB.WithContext(ctx).Create(p).Error
}
```
