# 📡 Step-by-Step Guide: Event-Driven Architecture & Outbox Pattern

This tutorial explains how to publish, subscribe, and guarantee message delivery with the SprinGo EventBus,
Transactional Outbox, and Dead Letter Queue (DLQ).

---

## 1. Overview

SprinGo provides an enterprise asynchronous messaging subsystem:
- **In-Memory & Distributed Pub/Sub**: Asynchronous event dispatching with goroutine pooling.
- **Transactional Outbox Pattern**: Persists domain events atomically alongside database changes.
- **Dead Letter Queue (DLQ)**: Retries failed event deliveries and captures permanently failing messages.
- **Actuator Web Console**: Inspect and re-dispatch failed events directly from the `/actuator/dashboard`.

---

## 2. Defining Domain Events

**Suggested File Path**: `internal/domain/event/order_created_event.go`
```go
package event

type OrderCreatedEvent struct {
    OrderID       uint    `json:"order_id"`
    CustomerEmail string  `json:"customer_email"`
    TotalAmount   float64 `json:"total_amount"`
}
```

---

## 3. Subscribing to Events

Subscribe to domain events using `event.Subscribe`:

**Suggested File Path**: `internal/infrastructure/input/events/order_event_listener.go`
```go
package events

import (
    "context"
    "log/slog"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/event"
    frameworkEvent "github.com/NeftaliAcosta/springo/framework/event"
)

func init() {
    frameworkEvent.Subscribe("order.created", handleOrderCreated)
}

func handleOrderCreated(ctx context.Context, payload event.OrderCreatedEvent) error {
    slog.Info("Processing new order event", "order_id", payload.OrderID)
    // Send email notification or trigger external webhook
    return nil
}
```

---

## 4. Publishing Events with Transactional Outbox

When persisting data inside a transaction, register the event so it is guaranteed to publish **only after commit**:

**Suggested File Path**: `internal/application/service/order_service.go`
```go
package service

import (
    "context"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/event"
    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
    "github.com/NeftaliAcosta/springo/framework/database"
    "gorm.io/gorm"
)

type OrderService struct {
    DB *gorm.DB `spring:"db"`
}

func (s *OrderService) PlaceOrder(ctx context.Context, order *model.Order) error {
    return database.Transactional(ctx, s.DB, func(txCtx context.Context) error {
        tx := database.GetTx(txCtx, s.DB)

        if err := tx.Create(order).Error; err != nil {
            return err
        }

        // Enqueue post-commit domain event
        database.RegisterPostCommitEvent(txCtx, "order.created", event.OrderCreatedEvent{
            OrderID:       order.ID,
            CustomerEmail: order.CustomerEmail,
            TotalAmount:   order.TotalAmount,
        })

        return nil
    })
}
```
