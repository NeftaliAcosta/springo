# 📝 Step-by-Step Guide: Structured Logging & Distributed Tracing

This tutorial explains how to use Go's standard `log/slog` structured logging and trace context propagation in SprinGo.

---

## 1. Overview

SprinGo integrates with Go 1.21+ `log/slog`:
- **Zero Global Mutation**: Clean dependency and context logging.
- **Trace & Request Context Correlation**: Automatically attaches `request_id`, `trace_id`, and `tenant_id` to log
  entries.
- **JSON & Text Formats**: Easily configured for local development text output or production JSON lines.
- **Log Level Filtering**: Dynamically controls `DEBUG`, `INFO`, `WARN`, and `ERROR` thresholds.

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
    // Standard structured log using context metadata
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
logging:
  level: info # Options: debug, info, warn, error
  format: json # Options: json, text
```

---

## 4. Output Log Format

```json
{
  "time": "2026-09-02T21:30:00.123Z",
  "level": "INFO",
  "msg": "Registering new user account",
  "request_id": "req-98fbc12a-3b12",
  "trace_id": "trace-4a88f01b",
  "email": "jane.doe@example.com",
  "action": "user_registration"
}
```
