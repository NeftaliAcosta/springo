# ⚙️ Step-by-Step Guide: Configuration Properties & Profiles

This tutorial explains how to manage application properties, multi-environment profiles, and custom configuration
structs in SprinGo.

---

## 1. Overview

SprinGo provides a Spring Boot-like configuration engine:
- **Zero-Boilerplate Binding**: Type-safe mapping from YAML files into Go structs via `config.RegisterProperties`
  and `config.Get[T]()`.
- **Environment Profiles**: Seamless switching with `SPRINGO_PROFILES_ACTIVE` (e.g. `local`, `dev`, `staging`, `prod`).
- **Dynamic Env Placeholders**: Supports Spring-style default fallbacks like `${DATABASE_URL:sqlite://data.db}`.
- **Fail-Fast Validation**: Automatic struct validation on application startup before servers open listeners.

---

## 2. Defining Configuration Properties

Create a struct matching your YAML hierarchy:

**Suggested File Path**: `internal/infrastructure/config/sentry_config.go`
```go
package config

import (
    "github.com/NeftaliAcosta/springo/framework/config"
)

// SentryProperties defines Sentry telemetry configuration in application.yaml
type SentryProperties struct {
    Enabled          bool    `yaml:"enabled"`
    DSN              string  `yaml:"dsn"`
    Environment      string  `yaml:"environment"`
    Release          string  `yaml:"release"`
    TracesSampleRate float64 `yaml:"traces-sample-rate"`
    Debug            bool    `yaml:"debug"`
}

func init() {
    // Register the property prefix 'management.sentry'
    config.RegisterProperties("management.sentry", &SentryProperties{
        Environment:      "local",
        TracesSampleRate: 1.0,
    })
}
```

---

## 3. Reading Configuration at Runtime

Retrieve the strongly typed configuration struct anywhere in your codebase using `config.Get[T]()`:

**Suggested File Path**: `internal/infrastructure/config/sentry_lifecycle.go`
```go
package config

import (
    "context"
    "log/slog"
    "strings"

    frameworkConfig "github.com/NeftaliAcosta/springo/framework/config"
    "github.com/NeftaliAcosta/springo/framework/lifecycle"
    "github.com/getsentry/sentry-go"
)

func init() {
    lifecycle.RegisterInitializer("observability.sentry", 10, func(ctx context.Context) error {
        return InitSentry()
    })
}

func InitSentry() error {
    cfg := frameworkConfig.Get[SentryProperties]()
    if cfg == nil || !cfg.Enabled || strings.TrimSpace(cfg.DSN) == "" {
        slog.Info("[Observability] Sentry is disabled or not configured")
        return nil
    }

    err := sentry.Init(sentry.ClientOptions{
        Dsn:              cfg.DSN,
        Environment:      cfg.Environment,
        Release:          cfg.Release,
        TracesSampleRate: cfg.TracesSampleRate,
        Debug:            cfg.Debug,
    })
    if err != nil {
        slog.Error("[Observability] Failed to initialize Sentry", "error", err)
        return err
    }

    slog.Info("[Observability] ✅ Sentry initialized successfully", "environment", cfg.Environment)
    return nil
}
```

---

## 4. Multi-Environment YAML Profiles

Place configuration files in `resources/`:
```text
resources/
├── application.yaml          # Base default configuration
├── application-local.yaml    # Local developer machine overrides
├── application-dev.yaml      # Remote development cluster overrides
└── application-prod.yaml     # Production hardened configuration
```

### Example: `resources/application.yaml`
```yaml
server:
  port: 8080
  api:
    base-path: /api/v1

spring:
  datasource:
    driver: sqlite
    url: ./app.db
    auto-migrate: true

management:
  sentry:
    enabled: false
    dsn: ${SENTRY_DSN:}
    environment: local
```

### Example: `resources/application-prod.yaml`
```yaml
server:
  port: 8080

spring:
  datasource:
    driver: postgres
    url: ${DATABASE_URL}
    auto-migrate: true

management:
  sentry:
    enabled: true
    dsn: ${SENTRY_DSN}
    environment: production
    traces-sample-rate: 0.2
```

---

## 5. Activating Profiles

Activate profiles using standard environment variables:

```bash
# Local development
SPRINGO_PROFILES_ACTIVE=local go run cmd/app/main.go

# Production Docker container / Kubernetes pod
export SPRINGO_PROFILES_ACTIVE=prod
./main
```
