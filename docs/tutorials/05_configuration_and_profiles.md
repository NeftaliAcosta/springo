# ⚙️ Step-by-Step Guide: Configuration Properties & Profiles

This tutorial explains how to manage application properties, multi-environment profiles, direct property reads,
and conditional configuration in SprinGo.

---

## 1. Overview

SprinGo provides a Spring Boot-like configuration engine:
- **Zero-Boilerplate Binding**: Type-safe mapping from YAML files into Go structs via `config.RegisterProperties`
  and `config.Get[T]()`.
- **Direct Property Lookups (@Value style)**: Lightweight accessors `GetString`, `GetInt`, `GetBool`, `GetDuration`,
  and `GetValue[T]` for reading single values without defining structs.
- **Conditional Configuration (@ConditionalOnProperty)**: Declarative callbacks `When`, `WhenProfile`, and
  `RegisterConditionalBean` executed during application startup.
- **Environment Profiles**: Seamless switching with `SPRINGO_PROFILES_ACTIVE` (e.g. `local`, `dev`, `prod`).
- **Dynamic Env Placeholders**: Supports Spring-style default fallbacks like `${DATABASE_URL:sqlite://data.db}`.
- **Fail-Fast Validation**: Automatic struct validation on application startup before servers open listeners.

---

## 2. Defining Configuration Properties

### Request optimization (optional)

SprinGo keeps the legacy request path by default. To enable pooled `RequestState` and HTTP response writers, add:

```yaml
server:
  features:
    request-optimization: true
```

The property is optional. Omitted or `false` preserves existing `context.WithValue()` access and application behavior.
The optimized path sanitizes pooled authentication state before reuse. Validate workload benchmarks before enabling it
in production.

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

## 3. Reading Configuration at Runtime (Typed Struct)

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

## 4. Direct Property Reading (@Value Helpers)

For isolated property lookups without declaring custom structs, use `@Value` style helpers:

```go
package service

import (
    "time"

    "github.com/NeftaliAcosta/springo/framework/config"
)

func ProcessOrder() {
    // Read string, boolean, integer, or duration with safe fallback defaults
    apiKey := config.GetString("payment.stripe.api-key", "default-test-key")
    isSandbox := config.GetBool("payment.stripe.sandbox", true)
    maxRetries := config.GetInt("payment.stripe.max-retries", 3)
    timeout := config.GetDuration("payment.stripe.timeout", 5*time.Second)

    // Read typed complex data structures
    type WebhookConfig struct {
        URL    string `yaml:"url"`
        Secret string `yaml:"secret"`
    }
    wh := config.GetValue[WebhookConfig]("payment.stripe.webhook", WebhookConfig{})
}
```

---

## 5. Declarative Conditional Configuration (@ConditionalOnProperty)

SprinGo allows registering initializers, callbacks, and beans conditionally during startup:

```go
package config

import (
    "github.com/NeftaliAcosta/springo/framework/config"
    "github.com/NeftaliAcosta/springo/framework/ioc"
)

func init() {
    // 1. Execute logic only when a property evaluates to expected value
    config.When("encryption.aes.enabled", true, func() {
        // Register encryption middleware or custom interceptors
    })

    // 2. Execute logic only for specific active profiles
    config.WhenProfile([]string{"prod", "staging"}, func() {
        // Register strict security checkers
    })

    // 3. Register beans conditionally in the IoC container
    config.RegisterConditionalBean("notificationService", "notifications.enabled", true, func() any {
        return &EmailNotificationService{}
    })
}
```

---

## 6. Multi-Environment YAML Profiles

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

## 7. Activating Profiles

Activate profiles using standard environment variables:

```bash
# Local development
SPRINGO_PROFILES_ACTIVE=local go run cmd/app/main.go

# Production Docker container / Kubernetes pod
export SPRINGO_PROFILES_ACTIVE=prod
./main
```
