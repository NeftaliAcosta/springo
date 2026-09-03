# 🔄 Step-by-Step Guide: Lifecycle Hooks & Graceful Shutdown

This tutorial explains how to manage application startup initialization, readiness verification, and clean graceful
shutdown in SprinGo.

---

## 1. Overview

SprinGo provides ordered application lifecycle callbacks inspired by Spring Boot's `SmartLifecycle`:
- **Initializers (`RegisterInitializer`)**: Execute fail-fast startup routines before HTTP listeners start.
- **Readiness Hooks (`RegisterReady`)**: Validate service health and external connections before signaling readiness.
- **Shutdown Hooks (`RegisterShutdown`)**: Execute in reverse order upon receiving `SIGINT` or `SIGTERM` signals.
- **Zero-Boilerplate `main.go`**: Keeps external library initialization out of `main.go`.

---

## 2. Registering Lifecycle Hooks

**Suggested File Path**: `internal/infrastructure/config/sentry_lifecycle.go`
```go
package config

import (
    "context"
    "log/slog"

    "github.com/NeftaliAcosta/springo/framework/lifecycle"
)

func init() {
    // 1. Initializer: runs in ascending order (order: 50)
    lifecycle.RegisterInitializer("sentry.init", 50, func(ctx context.Context) error {
        slog.Info("Initializing Sentry client...")
        return InitSentry()
    })

    // 2. Ready Hook: runs after HTTP listener is bound (order: 50)
    lifecycle.RegisterReady("sentry.ready", 50, func(ctx context.Context) error {
        slog.Info("Verifying Sentry readiness...")
        return nil
    })

    // 3. Shutdown Hook: runs in descending order during graceful shutdown
    lifecycle.RegisterShutdown("sentry.flush", 50, func(ctx context.Context) error {
        slog.Info("Flushing pending Sentry events before shutdown...")
        FlushSentry()
        return nil
    })
}
```

---

## 3. Minimal Application Entrypoint

**Suggested File Path**: `cmd/app/main.go`
```go
package main

import (
    "github.com/NeftaliAcosta/springo/framework"
)

func main() {
    // Bootstrap automatically runs all registered initializers, launches servers,
    // and listens for OS interrupt signals (SIGINT, SIGTERM) to execute graceful shutdown.
    framework.Bootstrap(framework.Options{}).Start()
}
```
