# 📝 Step-by-Step Guide: Structured Logging & Distributed Tracing

This tutorial explains how to use Go's standard `log/slog` structured logging, granular framework log level controls, banner configuration, and trace context propagation in SprinGo.

---

## 1. Overview

SprinGo integrates with Go 1.21+ `log/slog`:
- **Zero Global Mutation**: Clean dependency and context logging with structured attributes.
- **Granular Framework Log Control**: Separate application log level (`level`) and framework internal kernel log level (`framework-level: OFF | ERROR | WARN | INFO | DEBUG`).
- **Independent ASCII Banner Control**: Toggle startup banner independently via `spring.logging.show-banner: true | false`.
- **Trace & Request Context Correlation**: Automatically attaches `request_id`, `trace_id`, and `tenant_id` to log entries via `ContextHandler`.
- **JSON & Text Formats**: Easily configured for local development text output or production JSON lines.
- **Subsystem Routing**: Kernel components emit `subsystem: "springo"`, allowing developers to silence internal kernel messages during development or testing without suppressing user application logs.

---

## 2. Using Contextual Logging in Handlers and Services

**Suggested File Path**: `internal/application/service/user_service.go`
```go
package service

import (
    "context"
    "log/slog"
)

type UserService struct{}

func (s *UserService) RegisterUser(ctx context.Context, email string) error {
    // Standard structured log using context metadata (request_id, trace_id automatically attached)
    slog.InfoContext(ctx, "Registering new user account",
        "email", email,
        "action", "user_registration",
    )

    return nil
}
```

---

## 3. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  logging:
    level: info             # Application log level: DEBUG, INFO, WARN, ERROR
    framework-level: warn   # Framework internal log level: OFF, ERROR, WARN, INFO, DEBUG
    show-banner: true       # Toggle SprinGo ASCII identity banner (default: true)
    format: json            # Output format: json, text
    levels:                 # Per-package level overrides
      "github.com/myorg/myapp/internal/payment": debug
```

### Silencing Framework Logs in Unit/Integration Tests
To run clean tests without internal framework initialization logs:
```yaml
# resources/application-test.yaml
spring:
  logging:
    level: debug
    framework-level: "OFF"
    show-banner: false
```

---

## 4. Output Log Format

### Structured Application Log (JSON)
```json
{
  "time": "2026-09-05T00:30:00.123Z",
  "level": "INFO",
  "msg": "Registering new user account",
  "request_id": "req-98fbc12a-3b12",
  "trace_id": "trace-4a88f01b",
  "email": "jane.doe@example.com",
  "action": "user_registration"
}
```

### Framework Internal Log (Subsystem Tagged)
```json
{
  "time": "2026-09-05T00:30:00.124Z",
  "level": "INFO",
  "msg": "SprinGo Server running",
  "subsystem": "springo",
  "url": "http://localhost:8080"
}
```
