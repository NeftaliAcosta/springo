# 📊 Step-by-Step Guide: Custom Actuator Health Indicators & Telemetry

This tutorial explains how to register custom subsystem health checks, expose Prometheus metrics, and configure
the embedded Glassmorphic Actuator Dashboard in SprinGo.

---

## 1. Overview

SprinGo Actuator provides extensible observability primitives:
- **Custom Health Indicators**: Register domain/external dependency health checks with `web.RegisterHealthCheck`.
- **Automatic Status Discovery**: Automatically aggregates health statuses (`UP`, `DOWN`, `DEGRADED`).
- **Kubernetes Probe Privacy**: Minimal `{"status":"UP"}` on unauthenticated public requests and full component
  telemetry for authenticated admins.
- **Embedded Web Console**: View real-time component health at `http://localhost:8080/actuator/dashboard`.

---

## 2. Registering Custom Health Checks

**Suggested File Path**: `internal/infrastructure/config/health_indicators_lifecycle.go`
```go
package config

import (
    "context"
    "fmt"

    "github.com/NeftaliAcosta/springo/framework/lifecycle"
    "github.com/NeftaliAcosta/springo/framework/web"
)

func init() {
    lifecycle.RegisterInitializer("health.payment_gateway", 25, func(ctx context.Context) error {
        // Register custom health indicator for external payment gateway
        web.RegisterHealthCheck("paymentGateway", func(ctx context.Context) web.HealthComponent {
            err := checkStripePing(ctx)
            if err != nil {
                return web.HealthComponent{
                    Status: "DOWN",
                    Details: map[string]any{
                        "error": fmt.Sprintf("Payment provider unreachable: %v", err),
                    },
                }
            }

            return web.HealthComponent{
                Status: "UP",
                Details: map[string]any{
                    "provider": "Stripe API",
                    "latency":  "42ms",
                },
            }
        })

        return nil
    })
}
```

---

## 3. Configuration & Basic Auth

**Suggested File Path**: `resources/application.yaml`
```yaml
management:
  endpoints:
    web:
      exposure:
        include: "health,info,beans,goroutines,metrics,dlq"
      base-path: /actuator
  security:
    enabled: true
    username: admin
    password: ${ACTUATOR_PASSWORD:secret123}
```

---

## 4. Inspecting Health Endpoints

- **Public Unauthenticated Probe**:
  ```http
  GET /actuator/health
  200 OK
  {"status": "UP"}
  ```

- **Authenticated Detailed Health**:
  ```http
  GET /actuator/health (with Basic Auth)
  200 OK
  {
    "status": "UP",
    "components": {
      "db": {"status": "UP", "details": {"driver": "mysql", "open_connections": 4}},
      "redis": {"status": "UP", "details": {"ping": "PONG"}},
      "paymentGateway": {"status": "UP", "details": {"provider": "Stripe API", "latency": "42ms"}}
    }
  }
  ```
