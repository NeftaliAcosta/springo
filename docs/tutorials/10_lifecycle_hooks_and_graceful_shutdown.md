# 🔄 Step-by-Step Guide: Lifecycle Hooks & Graceful Shutdown

This tutorial explains how to manage application startup initialization, readiness verification, external telemetry
connections (e.g., Sentry, OpenTelemetry), and clean graceful shutdown in SprinGo.

---

## 1. Overview

SprinGo provides ordered application lifecycle callbacks inspired by Spring Boot's `SmartLifecycle`:
- **Initializers (`RegisterInitializer`)**: Execute fail-fast startup routines before HTTP listeners bind or worker pools launch.
- **Readiness Hooks (`RegisterReady`)**: Validate service health and external connections before signaling readiness.
- **Shutdown Hooks (`RegisterShutdown`)**: Execute in reverse order upon receiving `SIGINT` or `SIGTERM` signals.
- **Zero-Boilerplate `main.go`**: Keeps external library initialization and cleanup out of `main.go`.

---

## 2. Connecting Sentry via Lifecycle Hooks

In enterprise applications, error reporting tools like Sentry must be initialized before traffic arrives and flushed
cleanly during shutdown to avoid losing error events in flight.

### Configuration (`resources/application.yaml`)

```yaml
sentry:
  dsn: "${SENTRY_DSN:https://examplePublicKey@o0.ingest.sentry.io/0}"
  environment: "${SPRING_PROFILES_ACTIVE:local}"
  sample-rate: 1.0
  flush-timeout: 2s
```

### Implementation (`internal/infrastructure/config/sentry_lifecycle.go`)

```go
package config

import (
    "context"
    "fmt"
    "log/slog"
    "time"

    "github.com/NeftaliAcosta/springo/framework/config"
    "github.com/NeftaliAcosta/springo/framework/lifecycle"
    "github.com/NeftaliAcosta/springo/framework/logging"
    "github.com/getsentry/sentry-go"
)

type SentryProperties struct {
    Dsn          string        `yaml:"dsn"`
    Environment  string        `yaml:"environment"`
    SampleRate   float64       `yaml:"sample-rate"`
    FlushTimeout time.Duration `yaml:"flush-timeout"`
}

func init() {
    // 1. Initializer Hook (Order: 10) - Runs before HTTP server binds
    lifecycle.RegisterInitializer("sentry.init", 10, func(ctx context.Context) error {
        props := SentryProperties{
            SampleRate:   1.0,
            FlushTimeout: 2 * time.Second,
        }
        _ = config.Bind("sentry", &props)

        if props.Dsn == "" {
            slog.Info("Sentry DSN not configured, skipping telemetry initialization")
            return nil
        }

        err := sentry.Init(sentry.ClientOptions{
            Dsn:              props.Dsn,
            Environment:      props.Environment,
            TracesSampleRate: props.SampleRate,
            AttachStacktrace: true,
        })
        if err != nil {
            return fmt.Errorf("failed to initialize Sentry: %w", err)
        }

        slog.Info("✅ Sentry telemetry initialized successfully",
            slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
            slog.String("environment", props.Environment))
        return nil
    })

    // 2. Readiness Hook (Order: 10) - Verifies connectivity after server is up
    lifecycle.RegisterReady("sentry.ready", 10, func(ctx context.Context) error {
        slog.Info("🔍 Sentry reporter ready for production traffic")
        return nil
    })

    // 3. Shutdown Hook (Order: 10) - Flushes queued buffer before application exits
    lifecycle.RegisterShutdown("sentry.flush", 10, func(ctx context.Context) error {
        slog.Info("⏳ Flushing pending Sentry events before shutdown...",
            slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))

        flushed := sentry.Flush(2 * time.Second)
        if !flushed {
            slog.Warn("⚠️ Sentry flush timed out; some events may have been dropped",
                slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
        } else {
            slog.Info("✅ Sentry events flushed successfully",
                slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
        }
        return nil
    })
}
```

---

## 3. Minimal Application Entrypoint

With lifecycle hooks registered in `init()`, `main.go` remains purely declarative:

**Suggested File Path**: `cmd/app/main.go`
```go
package main

import (
    "github.com/NeftaliAcosta/springo/framework"
    _ "github.com/your-org/your-app/internal/infrastructure/config" // Import lifecycle hooks
)

func main() {
    // Bootstrap automatically runs all registered initializers, launches servers,
    // and listens for OS interrupt signals (SIGINT, SIGTERM) to execute graceful shutdown.
    framework.Bootstrap(framework.Options{}).Start()
}
```

