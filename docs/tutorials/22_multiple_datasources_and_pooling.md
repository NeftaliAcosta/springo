# 🗄️ Step-by-Step Guide: Multiple DataSources & Connection Pooling

This tutorial explains how to configure primary, secondary, and read-replica database connections with custom pool
tuning and session initialization in SprinGo.

---

## 1. Overview

SprinGo natively supports multi-database topologies:
- **Primary DataSource (`spring.datasource`)**: The default GORM connection used for migrations, IoC singletons,
  and transaction propagation.
- **Additional Named DataSources (`spring.additional-datasources`)**: Supplementary connections for read replicas,
  analytics warehouses, or multi-tenant databases.
- **Automated Health Monitoring (`health-check: true`)**: Automatically discovers and monitors connection liveness in
  the Actuator dashboard.
- **Configurable Connection Pool Tuning**: Granular control over maximum open/idle connections, connection lifetime,
  and idle timeouts with dialect-aware defaults.
- **Session Initialization (`session.init-sql`)**: Execute dialect-specific setup commands (e.g. setting timezones)
  upon opening connections.

---

## 2. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  # Primary Database (Read/Write)
  datasource:
    driver: postgres
    url: "${DATABASE_URL:postgres://postgres:secret@127.0.0.1:5432/app_db?sslmode=disable}"
    auto-migrate: true
    health-check: true
    migration-table: springo_migrations
    migration-lock-timeout: 5m
    pool:
      max-open-conns: 20          # default: 25 postgres/mysql, 10 sqlite file, 1 sqlite memory
      max-idle-conns: 10          # default: 10 postgres/mysql, 5 sqlite file, 1 sqlite memory
      conn-max-lifetime: 30m      # default: 30m postgres/mysql
      conn-max-idle-time: 5m      # default: 0 (unlimited)
      conn-timeout: 30s           # reserved timeout duration
    session:
      init-sql: "SET TIME ZONE 'UTC'" # executed upon establishing connection

  # Secondary Named DataSources
  additional-datasources:
    # Read-Only Replica
    readonly:
      driver: postgres
      url: "${DB_READONLY_URL:postgres://readonly:secret@127.0.0.1:5433/app_db?sslmode=disable}"
      health-check: true
      pool:
        max-open-conns: 50
        max-idle-conns: 25
        conn-max-lifetime: 15m

    # Analytics Warehouse (MySQL)
    analytics:
      driver: mysql
      url: "${ANALYTICS_DB_URL:user:pass@tcp(analytics-db:3306)/warehouse?parseTime=True}"
      health-check: true
      pool:
        max-open-conns: 10
        max-idle-conns: 5
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
