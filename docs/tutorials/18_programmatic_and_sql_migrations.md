# 🗄️ Step-by-Step Guide: SQL & Programmatic Go Migrations

This tutorial explains how to manage database schema evolutions using both raw SQL files and type-safe Go code with
`AutoMigrate` in SprinGo.

---

## 1. Overview

SprinGo provides a dual-engine database migration system:
- **Raw SQL Migrations**: Standard Flyway-style SQL scripts (`V1.0__create_table.sql`).
- **Programmatic Go Migrations**: Type-safe Go migrations with `db.AutoMigrate(&Entity{})` via `RegisterMigration`.
- **ShedLock Distributed Locking**: Prevents concurrent migration runs across multiple app replicas.
- **Checksum Integrity Validation**: Detects if historical migration files were modified after execution.
- **CLI Migration Commands**: `springo migrate`, `springo migrate status`, and `springo migrate rollback`.

---

## 2. Programmatic Go Migrations (`AutoMigrate`)

**Suggested File Path**: `resources/db/migration/20260818_000001_create_payment_currencies.go`
```go
package migration

import (
    "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/output/persistence/data"
    "github.com/NeftaliAcosta/springo/framework/database"
    "gorm.io/gorm"
)

func init() {
    database.RegisterMigration(database.Migration{
        Name: "20260818_000001_create_payment_currencies",
        Up: func(db *gorm.DB) error {
            // Automatically extracts schema from Go struct and runs DDL
            return db.AutoMigrate(&data.PaymentCurrencyEntity{})
        },
        Down: func(db *gorm.DB) error {
            return db.Migrator().DropTable(&data.PaymentCurrencyEntity{})
        },
    })
}
```

---

## 3. Raw SQL Migrations

**Suggested File Path**: `resources/db/migration/V1.1__create_users_table.sql`
```sql
CREATE TABLE IF NOT EXISTS users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    full_name VARCHAR(150) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
```

---

## 4. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  datasource:
    driver: postgres
    url: ${DATABASE_URL}
    auto-migrate: true # Automatically runs pending migrations on startup
    migration-table: springo_migrations
    migration-lock-timeout: 5m
```

---

## 5. Terminal CLI Controls

```bash
# Run pending migrations
springo migrate

# Inspect applied migration batches
springo migrate status

# Revert the latest migration batch
springo migrate rollback
```
