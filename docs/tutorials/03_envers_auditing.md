# 📜 Step-by-Step Guide: Hibernate Envers-Style Auditing in SprinGo

This tutorial explains how to implement automatic entity versioning and change auditing in SprinGo.

---

## 1. Overview

SprinGo provides automated change data capture inspired by **Hibernate Envers**:
- **Automatic Audit Tables**: Generates dynamic `${table}_aud` tables matching target SQL dialects.
- **Full History Tracking**: Records every `INSERT` (`ADD`), `UPDATE` (`MOD`), and `DELETE` (`DEL`).
- **User Context Attribution**: Extracts request user from JWT/session context with sanitization against log injection.
- **Dialect Native DDL**: Supports PostgreSQL (`BIGSERIAL`, `TIMESTAMPTZ`), MySQL (`AUTO_INCREMENT`), and SQLite.

---

## 2. Marking Models as Audited

Add the `springo:"audited"` struct tag to your GORM model:

**Suggested File Path**: `internal/domain/model/product.go`
```go
package model

type Product struct {
    ID          uint    `gorm:"primaryKey"`
    Name        string  `gorm:"size:200"`
    Price       float64
    SprinGo     string  `springo:"audited" gorm:"-"`
}
```

---

## 3. Enabling Auditing in Application Setup

Register models with `database.EnableAuditing`:

**Suggested File Path**: `internal/infrastructure/config/audit_lifecycle.go`
```go
package config

import (
    "context"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
    "github.com/NeftaliAcosta/springo/framework/config"
    "github.com/NeftaliAcosta/springo/framework/database"
    "github.com/NeftaliAcosta/springo/framework/lifecycle"
)

func init() {
    lifecycle.RegisterInitializer("database.audit", 20, func(ctx context.Context) error {
        db, err := database.Connect(config.Get[database.DataSourceProperties]())
        if err != nil {
            return err
        }

        return database.EnableAuditing(db, &model.Product{})
    })
}
```

---

## 4. Attributing the Acting User

SprinGo inspects the request context for user identifiers (`username`, `sub`, `email`, `user_id`):

**Suggested File Path**: `internal/infrastructure/input/rest/product_controller.go`
```go
package rest

import (
    "context"
)

func (c *ProductController) updatePrice(ctx context.Context, id uint, price float64) error {
    // Context contains user from JWT middleware
    // rev_user will automatically be set to the authenticated username in products_aud
    return c.service.UpdatePrice(ctx, id, price)
}
```

### Generated Audit Schema (`products_aud`)
| Column | Type | Description |
| :--- | :--- | :--- |
| `audit_id` | `BIGSERIAL` / `INT AUTO_INCREMENT` | Primary key of the audit revision. |
| `id` | `uint` | Entity ID being audited. |
| `name` | `string` | Snapshot value of entity name. |
| `price` | `float64` | Snapshot value of entity price. |
| `rev_type` | `VARCHAR(10)` | Revision type: `ADD`, `MOD`, or `DEL`. |
| `rev_user` | `VARCHAR(255)` | Sanitized username / actor ID. |
| `rev_timestamp` | `TIMESTAMPTZ` / `DATETIME` | UTC timestamp of the revision. |
