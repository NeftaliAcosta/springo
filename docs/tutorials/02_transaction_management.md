# ⚡ Step-by-Step Guide: Transaction Management in SprinGo

This tutorial explains how to manage database transactions, read-only optimization, and propagation levels using SprinGo.

---

## 1. Overview

SprinGo adapts Spring Boot's declarative `@Transactional` mechanics to idiomatic Go:
- **Zero-Boilerplate Execution**: Automatic commit and rollback handling with `database.Transactional` and `database.RunInTx[T]`.
- **Read-Only Transaction Optimization**: Spring-like `@Transactional(readOnly = true)` via `database.WithReadOnly()`.
- **Panic & Error Safety**: Automatically captures runtime panics and GORM internal errors to execute physical rollback.
- **Propagation Levels**: Full support for 7 Spring-like transaction propagation modes.
- **Context Propagation**: Transaction state, read-only flags, and post-commit domain events are propagated seamlessly in
  `context.Context`.

---

## 2. Basic Transaction Usage

**Suggested File Path**: `internal/application/service/account_service.go`
```go
package service

import (
    "context"
    "fmt"

    "github.com/NeftaliAcosta/springo/framework/database"
    "gorm.io/gorm"
)

type AccountService struct {
    DB *gorm.DB
}

func (s *AccountService) TransferFunds(
    ctx context.Context,
    fromID uint,
    toID uint,
    amount float64,
) error {
    return database.Transactional(ctx, func(txCtx context.Context) error {
        // Extract the active transaction from context (or fallback to s.DB)
        tx := database.GetTx(txCtx, s.DB)

        if err := tx.Model(&Account{}).Where("id = ?", fromID).
            Update("balance", gorm.Expr("balance - ?", amount)).Error; err != nil {
            return fmt.Errorf("debit failed: %w", err)
        }

        if err := tx.Model(&Account{}).Where("id = ?", toID).
            Update("balance", gorm.Expr("balance + ?", amount)).Error; err != nil {
            return fmt.Errorf("credit failed: %w", err)
        }

        return nil
    })
}
```

---

## 3. Read-Only Transactions (`@Transactional(readOnly = true)`)

SprinGo provides native read-only transaction optimization through `database.WithReadOnly()`:

**Suggested File Path**: `internal/application/service/report_service.go`
```go
package service

import (
    "context"

    "github.com/NeftaliAcosta/springo/framework/database"
    "gorm.io/gorm"
)

type ReportService struct {
    DB *gorm.DB `spring:"db"`
}

// GetAccountSummary runs inside an optimized read-only transaction scope
func (s *ReportService) GetAccountSummary(ctx context.Context, accountID uint) (*AccountSummary, error) {
    return database.RunInTx(ctx, func(txCtx context.Context) (*AccountSummary, error) {
        tx := database.GetTx(txCtx, s.DB)

        var summary AccountSummary
        if err := tx.Model(&Account{}).Where("id = ?", accountID).First(&summary).Error; err != nil {
            return nil, err
        }

        return &summary, nil
    }, database.WithReadOnly())
}
```

### Multi-Dialect Driver Behavior:
- **PostgreSQL**: Issues `SET TRANSACTION READ ONLY` immediately after transaction begin. Optimizes query planning and strictly prevents accidental writes.
- **MySQL**: Issues `SET TRANSACTION READ ONLY`, avoiding undo log allocation for write transactions.
- **SQLite**: Safely handled as a no-op without syntax error.
- **Context Inspection**: Use `database.IsTxReadOnly(ctx)` to check whether the current transactional context is marked as read-only.

---

## 4. Transaction Propagation Modes

Specify propagation via `database.WithPropagation`:

| Propagation Level | Description |
| :--- | :--- |
| `PropagationRequired` *(default)* | Joins existing transaction if present, or creates a new one. |
| `PropagationRequiresNew` | Suspends active transaction and creates an independent transaction. |
| `PropagationNested` | Executes inside a savepoint if active transaction exists. |
| `PropagationSupports` | Executes inside active transaction if present; otherwise runs non-transactionally. |
| `PropagationNotSupported` | Suspends active transaction and executes non-transactionally. |
| `PropagationMandatory` | Requires an active transaction; returns error if none exists. |
| `PropagationNever` | Requires no active transaction; returns error if active transaction exists. |

### Example: Nested `RequiresNew` inside Read-Only Scope
When an outer transaction is `readOnly` and an inner operation requires writing (such as audit logging), `PropagationRequiresNew` starts an independent read-write physical transaction while preserving the outer read-only constraint:

**Suggested File Path**: `internal/application/service/audit_service.go`
```go
package service

import (
    "context"

    "github.com/NeftaliAcosta/springo/framework/database"
    "gorm.io/gorm"
)

type AuditService struct {
    DB *gorm.DB `spring:"db"`
}

func (s *AuditService) LogSecurityEvent(ctx context.Context, msg string) error {
    return database.Transactional(
        ctx,
        func(txCtx context.Context) error {
            tx := database.GetTx(txCtx, s.DB)
            return tx.Create(&SecurityLog{Message: msg}).Error
        },
        database.WithPropagation(database.PropagationRequiresNew),
        database.WithReadOnly(false), // Explicit read-write independent transaction
    )
}
```

---

## 5. Post-Commit Domain Events

SprinGo guarantees that domain events registered inside a transaction are published **only after successful commit**:

**Suggested File Path**: `internal/application/service/order_service.go`
```go
package service

import (
    "context"

    "github.com/NeftaliAcosta/springo/framework/database"
    "gorm.io/gorm"
)

type OrderService struct {
    DB *gorm.DB `spring:"db"`
}

func (s *OrderService) CreateOrder(ctx context.Context, order *Order) error {
    return database.Transactional(ctx, func(txCtx context.Context) error {
        tx := database.GetTx(txCtx, s.DB)
        if err := tx.Create(order).Error; err != nil {
            return err
        }

        // Event will only dispatch if transaction commits successfully
        database.AddEventToTransaction(txCtx, OrderCreatedEvent{OrderID: order.ID})
        return nil
    })
}
```

