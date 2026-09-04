# 🧩 Step-by-Step Guide: Bean Configuration & IoC in SprinGo

This tutorial explains how to register, configure, and inject dependencies using the SprinGo IoC container.

---

## 1. Overview

SprinGo provides a thread-safe, Spring-inspired Inversion of Control (IoC) container with:
- **Singleton & Prototype Lifecycle**: Caches singleton bean instances by default with `sync.Once`.
- **Factory Functions**: Supports factory constructors returning `T` or `(T, error)` with automatic error wrapping.
- **Dynamic Parameter Injection**: Automatically resolves factory parameters from registered beans.
- **Provider Pattern**: Type-safe lazy resolution via `ioc.Provider[T]` and pointer `*ioc.Provider[T]`.
- **Tag-based Autowiring**: Automatically injects dependencies into struct fields marked with `spring:"beanName"`.

---

## 2. Registering Beans

### 2.1 Direct Instance Registration
**Suggested File Path**: `internal/infrastructure/config/email_service_config.go`
```go
package config

import (
    "github.com/NeftaliAcosta/springo/framework/ioc"
)

type EmailService struct {
    SMTPHost string
}

func init() {
    ioc.RegisterBean("emailService", &EmailService{SMTPHost: "smtp.example.com"})
}
```

### 2.2 Factory Function with Error Handling `(T, error)`
**Suggested File Path**: `internal/infrastructure/config/database_client_config.go`
```go
package config

import (
    "github.com/NeftaliAcosta/springo/framework/ioc"
)

func init() {
    ioc.RegisterFactory("databaseClient", func() (*Client, error) {
        client, err := NewClient("connection-string")
        if err != nil {
            return nil, err
        }
        return client, nil
    })
}
```

### 2.3 Factory with Automatic Parameter Injection
**Suggested File Path**: `internal/infrastructure/config/order_service_config.go`
```go
package config

import (
    "github.com/NeftaliAcosta/springo/framework/ioc"
)

// SprinGo automatically resolves 'emailService' and 'databaseClient' by type or name
func init() {
    ioc.RegisterFactory("orderService", func(es *EmailService, db *Client) *OrderService {
        return &OrderService{
            Email: es,
            DB:    db,
        }
    })
}
```

### 2.4 Conditional Bean Registration (@ConditionalOnProperty)
**Suggested File Path**: `internal/infrastructure/config/notification_service_config.go`
```go
package config

import (
    "github.com/NeftaliAcosta/springo/framework/config"
)

func init() {
    // Only registers when 'features.notifications.enabled' is true in YAML/env
    config.RegisterConditionalBean("notificationService", "features.notifications.enabled", true, func() any {
        return &EmailNotificationService{}
    })
}
```

---

## 3. Injecting Dependencies

### 3.1 Tag-based Field Autowiring
**Suggested File Path**: `internal/infrastructure/input/rest/order_controller.go`
```go
package rest

import (
    "github.com/NeftaliAcosta/springo/framework/ioc"
)

type OrderController struct {
    OrderService *OrderService `spring:"orderService"`
}

func init() {
    controller := &OrderController{}
    ioc.RegisterBean("orderController", controller)
    // Dependencies marked with spring:"..." are automatically wired
}
```

### 3.2 Lazy Injection via `ioc.Provider[T]`
**Suggested File Path**: `internal/application/service/report_generator.go`
```go
package service

import (
    "github.com/NeftaliAcosta/springo/framework/ioc"
)

type ReportGenerator struct {
    BillingProvider ioc.Provider[*BillingService] `spring:"billingService"`
}

func (r *ReportGenerator) Generate() {
    // Resolved only when Get() is called
    billing := r.BillingProvider.Get()
    billing.Process()
}
```

---

## 4. Retrieving Beans Programmatically
**Suggested File Path**: `cmd/app/main.go` or `internal/application/service/startup.go`
```go
package main

import (
    "log"

    "github.com/NeftaliAcosta/springo/framework/ioc"
)

func initialize() {
    // Retrieve by name and type
    orderService, err := ioc.Get[OrderService]("orderService")
    if err != nil {
        log.Fatalf("Failed to resolve orderService: %v", err)
    }
    _ = orderService
}
```
