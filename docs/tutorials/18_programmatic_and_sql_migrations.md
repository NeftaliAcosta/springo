# 🗄️ Step-by-Step Guide: SQL & Programmatic Go Migrations

This tutorial explains how to manage database schema evolutions using automatic SQL file loading, Flyway-style naming
conventions, baseline support for existing databases, and programmatic Go migrations with `AutoMigrate` in SprinGo.

---

## 1. Overview

SprinGo provides a unified, dual-engine database migration system:
- **Auto-Loaded Raw SQL Migrations**: Directory scanning for Flyway-style SQL files (`V1__init.sql`, `V2.1__index.sql`).
- **Programmatic Go Migrations**: Type-safe Go migrations with `db.AutoMigrate(&Entity{})` via `RegisterMigration`.
- **Natural Version Sorting**: Semantic segment ordering (`V1 < V2 < V10 < V52`) preventing alphabetical sorting bugs.
- **Baseline Support (`baseline-on-migrate`)**: Initialize control tables on pre-existing databases without running DDL.
- **SHA-256 Checksum Integrity**: Automatically detects if historical SQL or Go migrations were tampered with post-execution.
- **Distributed Lock Safety**: Prevents race conditions during simultaneous startup across multiple Kubernetes pods.
- **CLI Management**: `springo migrate`, `springo migrate status`, `springo migrate rollback`, `reset`, and `refresh`.

---

## 2. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  datasource:
    driver: postgres
    url: "${DATABASE_URL:postgres://postgres:secret@127.0.0.1:5432/app_db?sslmode=disable}"
    auto-migrate: true # Automatically runs pending migrations on application bootstrap
    migration:
      table: springo_migrations          # Control table name (default: springo_migrations)
      lock-timeout: 5m                   # Distributed lock expiration duration
      locations:
        - resources/db/migration         # Directories scanned for .sql files
      sql-prefix: V                      # Prefix for versioned SQL migrations (default: V)
      baseline-on-migrate: false         # If true, marks existing schema up to baseline-version as applied
      baseline-version: "0"              # Cutoff version for baseline
      out-of-order: false                # Reject older migrations added after newer ones were applied
```

---

## 3. SQL File Migrations (Auto-Discovered)

Place versioned `.sql` files inside `resources/db/migration/`.

### Naming Conventions:
- Version format: `V{version}__{description}.sql` (e.g. `V1__create_users_table.sql`, `V2.1__add_email_index.sql`).
- Timestamp format: `YYYYMMDD_HHMMSS__{description}.sql` (e.g. `20260904_000001__seed_permissions.sql`).

**Suggested File Path**: `resources/db/migration/V1__create_users_table.sql`
```sql
CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    email VARCHAR(255) NOT NULL UNIQUE,
    full_name VARCHAR(150) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ DEFAULT CURRENT_TIMESTAMP
);
```

### Optional Rollback Scripts:
Create a matching `.undo.sql` file (e.g. `V1__create_users_table.undo.sql` or `U1__create_users_table.sql`):
```sql
DROP TABLE IF EXISTS users;
```

---

## 4. Programmatic Go Migrations (`RegisterMigration`)

When schema changes require data transformation or Go logic, register programmatic migrations in `init()`:

**Suggested File Path**: `resources/db/migration/20260904_000002_seed_admin.go`
```go
package migration

import (
    "github.com/NeftaliAcosta/springo/framework/database"
    "gorm.io/gorm"
)

func init() {
    database.RegisterMigration(database.Migration{
        Name: "20260904_000002_seed_admin",
        Up: func(db *gorm.DB) error {
            return db.Exec("INSERT INTO users (email, full_name) VALUES (?, ?)", "admin@example.com", "Administrator").Error
        },
        Down: func(db *gorm.DB) error {
            return db.Exec("DELETE FROM users WHERE email = ?", "admin@example.com").Error
        },
    })
}
```

---

## 5. Baseline for Existing Databases

When connecting SprinGo to a database that already contains tables created outside the migration pipeline:

```yaml
spring:
  datasource:
    migration:
      baseline-on-migrate: true
      baseline-version: "V2.0" # Mark all migrations <= V2.0 as applied without executing DDL
```

---

## 6. Terminal CLI Commands

### Scaffolding Migrations

```bash
# Generate programmatic Go migration (.go)
springo make migration CreateUsersTable

# Generate Flyway-style SQL migration (.sql + .undo.sql)
springo make migration CreateUsersTable --sql
```

### Running and Rolling Back Migrations

```bash
# Run all pending SQL and Go migrations
springo migrate

# Inspect migration batches, execution status, and applied timestamps
springo migrate status

# Rollback the last migration batch
springo migrate rollback

# Rollback a specific number of migrations
springo migrate rollback --steps=2

# Reset all migrations
springo migrate reset

# Reset and re-apply all migrations from scratch
springo migrate refresh
```
