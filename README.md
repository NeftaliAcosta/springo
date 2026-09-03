# SprinGo Framework 🚀

<p align="center">
  <b>High-performance, opinionated Go framework for Spring Boot developers.</b>
</p>

<p align="center">
  <a href="https://github.com/NeftaliAcosta/springo/actions/workflows/ci.yml"><img src="https://github.com/NeftaliAcosta/springo/actions/workflows/ci.yml/badge.svg" alt="CI Status"></a>
  <a href="https://pkg.go.dev/github.com/NeftaliAcosta/springo"><img src="https://pkg.go.dev/badge/github.com/NeftaliAcosta/springo.svg" alt="Go Reference"></a>
  <a href="https://github.com/NeftaliAcosta/springo/releases"><img src="https://img.shields.io/github/v/release/NeftaliAcosta/springo?include_prereleases&color=blue" alt="Release"></a>
  <a href="https://goreportcard.com/report/github.com/NeftaliAcosta/springo"><img src="https://goreportcard.com/badge/github.com/NeftaliAcosta/springo" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>

<p align="center">
  <img src="https://img.shields.io/github/stars/NeftaliAcosta/springo?style=social" alt="Stars">
  <img src="https://img.shields.io/github/forks/NeftaliAcosta/springo?style=social" alt="Forks">
  <img src="https://img.shields.io/github/issues/NeftaliAcosta/springo" alt="Issues">
  <img src="https://img.shields.io/github/contributors/NeftaliAcosta/springo" alt="Contributors">
</p>

---

> ⚠️ **Release status:** `v1.0.0-rc17` is a Release Candidate. Validate it in a staging environment before adopting it for production workloads; public APIs may still receive release-blocking corrections before `v1.0.0`.

---

## ⚡ Quick Start

### 1. Install SprinGo CLI

```bash
go install github.com/NeftaliAcosta/springo/cmd/springo@v1.0.0-rc17
```

### 2. Scaffold a New Enterprise Service

```bash
springo new my-service
cd my-service
go run cmd/app/main.go
```

Your API is now live at `http://localhost:8080` with Actuator Dashboard at `http://localhost:8080/actuator/dashboard`! 🎉

---

## 🚀 Key Features

| Category | Feature | Description |
| :--- | :--- | :--- |
| 🛠️ **CLI Tooling** | **Code Generators & Scaffolding** | `springo new` and `springo make` for instant Hexagonal Architecture components. |
| 🎛️ **Management** | **Spring-style Actuator** | Embedded Glassmorphic UI Dashboard for Health, Goroutine Dumps, Beans & DLQ. |
| 🧩 **Core Engine** | **IoC & Auto-Wiring** | Dependency injection container with reflection & tag-based field autowiring (`spring:"beanName"`). |
| 🔄 **Lifecycle** | **Initializer, Ready & Shutdown Hooks** | Ordered, fail-safe application hooks keep infrastructure setup out of `main.go`. |
| 📤 **Web Binding** | **JSON & Multipart DTOs** | Declarative request binding for JSON, path/query values and streamed multipart files with configurable limits. |
| 🔒 **Security** | **Enterprise JWT & CSRF** | Support for HS256, RS256 (Keycloak/Auth0 JWKS), OWASP Security Headers & CSRF. |
| ⚡ **Database** | **GORM & ShedLock** | Declarative transactions with `REQUIRED` propagation and cluster-wide cron locking. |
| 📡 **Messaging** | **Event Bus & Outbox/DLQ** | Domain Pub/Sub with Outbox buffer, automatic retries, and Dead Letter Queue management. |
| ⚙️ **Config** | **Profiles & Validation** | Fail-fast property validation with `application-{profile}.yaml` environments. |

---

## 🏗️ Architecture

SprinGo enforces a **Flattened Professional Hexagonal Architecture** with a dedicated **Kernel**:

```text
springo/
├── framework/                 # 🛠️ THE KERNEL (SprinGo Engine Library)
│   ├── app.go
│   ├── runner.go
│   ├── cache/                 # Multi-provider cache abstraction
│   ├── config/                # YAML Profile loader & validation
│   ├── database/              # GORM datasource & ShedLock migrator
│   ├── event/                 # Pub/Sub EventBus, Outbox & DLQ
│   ├── ioc/                   # Dependency Injection Container
│   ├── lifecycle/             # Ordered startup, readiness & shutdown hooks
│   ├── scheduler/             # Cron manager with ShedLock
│   ├── security/              # JWT & LDAP providers
│   └── web/                   # Chi router, Actuator & Validation
├── cmd/
│   ├── cli/                   # 🛠️ SprinGo CLI implementation
│   └── springo/               # Installable `springo` entrypoint (v1.0.0-rc17)
├── demo-api/                  # 🚀 Reference Application
└── README.md
```

---

## 🛠️ SprinGo CLI (`springo`)

The `springo` CLI automates daily development workflows:

### Scaffolding & Code Generation

```bash
# 1. Create a new microservice
springo new billing-service

# 2. Generate Hexagonal domain components
springo make model Invoice
springo make dto Invoice
springo make repository Invoice
springo make service Invoice
springo make controller Invoice
springo make migration CreateInvoicesTable
```

### Database Migrations & Route Discovery

```bash
# Migration controls
springo migrate
springo migrate status
springo migrate rollback --steps=1

# Terminal route discovery
springo routes

# Regenerate OpenAPI/Swagger documentation when controllers or DTOs change
springo swagger
```

Controllers registrados con `web.Dispatch` pueden recibir `http.ResponseWriter` para adaptar headers o cookies
sin mover detalles HTTP al application service:

```go
func (c *AuthController) login(
    ctx context.Context,
    writer http.ResponseWriter,
    req request.LoginRequestDTO,
) (any, error) {
    http.SetCookie(writer, refreshCookie)
    return c.authUseCase.Login(ctx, req.Email, req.Password)
}
```

Cuando un `POST` exitoso conserva semántica `200 OK`, declarar
`web.Dispatch(c.login, web.WithSuccessStatus(http.StatusOK))`. Sin override, `POST` continúa respondiendo `201`.

`springo run` keeps hot reload fast by compiling only the application. Swagger generation is intentionally explicit because dependency-aware documentation analysis is considerably slower than an incremental Go build. Use `springo swagger --quiet` for silent generation or `springo swagger --main path/to/main.go` for a custom entry point.

---

## 🔄 Application Lifecycle

Infrastructure components can register ordered lifecycle hooks instead of adding setup and cleanup logic to
`main.go`. This follows the same separation used by Spring Boot lifecycle callbacks while keeping Go registration
explicit and type-safe.

```go
package observability

import (
    "context"

    "github.com/NeftaliAcosta/springo/framework/lifecycle"
)

func init() {
    lifecycle.RegisterInitializer("observability.sentry", 100, initializeSentry)
    lifecycle.RegisterReady("observability.sentry", 100, verifySentry)
    lifecycle.RegisterShutdown("observability.sentry", 100, flushSentry)
}

func initializeSentry(ctx context.Context) error {
    // Load and validate the integration after configuration and IoC are available.
    return nil
}

func verifySentry(ctx context.Context) error {
    // Optionally verify readiness after the HTTP listener has been created.
    return nil
}

func flushSentry(ctx context.Context) error {
    // Flush and close the integration during graceful shutdown.
    return nil
}
```

The application entrypoint remains focused on composition:

```go
func main() {
    framework.Bootstrap(framework.Options{
        Middlewares: []func(http.Handler) http.Handler{
            web.SecurityHeadersMiddleware,
        },
    }).Start()
}
```

- Hook names must be non-empty and unique within their lifecycle phase.
- Initializers run after configuration, datasources, migrations, and IoC initialization. They execute in ascending
  `order` and fail fast, so `BootstrapE` returns the error and performs registered cleanup.
- Ready hooks run in ascending `order` after the HTTP listener is created but before `Application.Ready()` is
  signaled. All errors are collected; any error prevents readiness and triggers graceful shutdown.
- Shutdown hooks run once during `Application.Shutdown`, in descending `order`, so resources close in reverse order.
  All errors are collected without skipping later hooks.
- `Application.Start()` handles `SIGINT` and `SIGTERM`, then invokes graceful shutdown before the process exits.
- Each hook receives a `context.Context`. A panic is recovered and returned as a named lifecycle error.

Use `lifecycle.BackupRegistrations()` only in tests to isolate global registrations and restore them afterward.

---

## 📊 Actuator Dashboard

SprinGo includes an embedded, zero-dependency Web Console inspired by **Spring Boot Admin**:

- **Health Engine**: Automatic discovery and status checks for all GORM datasources & Redis connections.
- **Goroutine Dump**: Real-time stack trace inspection for concurrency debugging.
- **Bean Directory**: Full visibility into active IoC container definitions.
- **Dead Letter Queue**: Web interface to inspect, retry (re-dispatch), or purge failed domain events.

Access it locally at: `http://localhost:8080/actuator/dashboard`

---

## ⚙️ Configuration & Profiles

Configuration is **100% optional**. If omitted, SprinGo falls back to sensible enterprise defaults. Load specific profile files via `SPRINGO_PROFILES_ACTIVE`:

Application routes use `/api/v1` by default. Override the prefix without changing the kernel:

```yaml
server:
  api:
    base-path: /platform/v2
```

Use `/` to expose application routes without a common prefix. The value must begin with `/`; trailing slashes are normalized. Keep the Swagger `@BasePath` annotation and JWT `public-paths` synchronized with a custom prefix.

### Multipart file binding

SprinGo binds `multipart/form-data` directly to request DTOs while preserving path and query binding:

```go
type UploadRequest struct {
    ResourceUUID string             `path:"resource_uuid" validate:"required,uuid"`
    Description  string             `form:"description"`
    File         *web.MultipartFile `form:"file" validate:"required"`
}

func (c *ResourceController) upload(ctx context.Context, dto UploadRequest) (any, error) {
    file, err := dto.File.Open()
    if err != nil {
        return nil, err
    }
    defer file.Close()
    return c.service.Upload(ctx, dto.ResourceUUID, dto.File.Filename, file)
}
```

Configure global upload limits in bytes. Content above `memory-threshold` is spooled to temporary storage and cleaned automatically after the controller returns:

```yaml
server:
  multipart:
    enabled: true
    max-file-size: 104857600
    max-request-size: 115343360
    memory-threshold: 8388608
```

```bash
# Development profile (loads resources/application-dev.yaml)
SPRINGO_PROFILES_ACTIVE=dev go run cmd/app/main.go

# Production profile (loads resources/application-prod.yaml)
SPRINGO_PROFILES_ACTIVE=prod ./main
```

---

## 📖 Step-by-Step Tutorials & Documentation

Explore our comprehensive library of step-by-step guides from zero to production:

### 🚀 Getting Started & CLI
- 🚀 **[Zero-to-Production Beginner Guide](docs/tutorials/11_zero_to_hero_quickstart.md)**:
  Scaffolding, architecture, database setup, Docker containerization, and cloud deployment.
- 🛠️ **[SprinGo CLI Complete Reference](docs/tutorials/06_springo_cli_complete_guide.md)**:
  All commands, generators, database migrations, route discovery, and Swagger tools.
- 🏗️ **[Flattened Hexagonal Architecture Guide](docs/tutorials/19_hexagonal_architecture_pattern.md)**:
  Clean separation of concerns, domain models, ports, services, and adapters.
- ☕ **[Java Spring Boot to SprinGo Migration Guide](docs/tutorials/17_spring_boot_migration_guide.md)**:
  Rosetta Stone mapping annotations, concepts, and architectural patterns to Go.

### 🧩 Core Framework & IoC
- ⚙️ **[Configuration Properties & Profiles](docs/tutorials/05_configuration_and_profiles.md)**:
  Fail-fast YAML binding, dynamic env fallbacks, Sentry/Redis setups, and multi-profile environments.
- 🧩 **[IoC Container & Bean Configuration](docs/tutorials/01_bean_configuration.md)**:
  Factories `(T, error)`, dynamic parameter injection, `Provider[T]`, and field autowiring.
- 🔄 **[Lifecycle Hooks & Graceful Shutdown](docs/tutorials/10_lifecycle_hooks_and_graceful_shutdown.md)**:
  Ordered startup initializers, readiness verification, and OS signal shutdown traps.

### 🌐 Web, REST & Security
- 🌐 **[REST Web Routing & DTO Validation](docs/tutorials/07_web_routing_and_validation.md)**:
  Chi router integration, `web.Dispatch`, JSON & Multipart file binding, and status overrides.
- 🛡️ **[Advanced DTO Validation & Groups](docs/tutorials/23_advanced_dto_validation_and_groups.md)**:
  Validation groups (`OnCreate`, `OnUpdate`), custom validator tags, and Problem Details.
- 🌐 **[CORS & Origin Whitelist Configuration](docs/tutorials/21_cors_whitelist_configuration.md)**:
  Exact origin whitelist, dynamic wildcard patterns, credentials, and preflight caching.
- 🔐 **[JWT Security, OWASP Headers & Middleware](docs/tutorials/08_security_jwt_and_middleware.md)**:
  JWT (HS256/RS256 JWKS), claims extraction, security headers, CSRF, and custom middlewares.
- 🏢 **[Corporate Security & Active Directory LDAP](docs/tutorials/20_corporate_security_and_ldap.md)**:
  Active Directory / LDAP authentication, group-to-role mappings, and TLS/StartTLS.
- 🚨 **[Standardized Error Handling & RFC 7807](docs/tutorials/14_error_handling_and_rfc7807.md)**:
  Domain error sentinels, Problem Details output, and field-level validation errors.

### ⚡ Data, Transactions & Background Jobs
- ⚡ **[Declarative Transaction Management](docs/tutorials/02_transaction_management.md)**:
  Spring-like propagation levels, panic/error rollback safety, and post-commit events.
- 🗄️ **[Multiple DataSources & Pool Tuning](docs/tutorials/22_multiple_datasources_and_pooling.md)**:
  Primary and named secondary datasources, read replicas, and connection pool sizing.
- 🗄️ **[SQL & Programmatic Go Migrations](docs/tutorials/18_programmatic_and_sql_migrations.md)**:
  Dual-engine migrations with `db.AutoMigrate(&Entity{})`, Flyway-style SQL, and ShedLock.
- 📜 **[Hibernate Envers-Style Auditing](docs/tutorials/03_envers_auditing.md)**:
  Change data capture, dialect-native DDLs, and user context sanitization.
- 📦 **[Cache Abstraction & Redis Integration](docs/tutorials/12_caching_and_redis.md)**:
  Multi-driver cache engine, TTL expirations, and Actuator health integration.
- ⏰ **[Distributed Scheduling & ShedLock](docs/tutorials/13_distributed_scheduler_and_shedlock.md)**:
  Cluster-safe cron tasks, database locking, and multi-replica safety.
- 📡 **[Event-Driven Architecture & Outbox](docs/tutorials/09_event_driven_architecture.md)**:
  Domain Pub/Sub EventBus, Transactional Outbox, automatic retries, and Dead Letter Queue.

### 📊 Observability & Testing
- 📊 **[Actuator Diagnostics & Observability](docs/tutorials/04_actuator_observability.md)**:
  Glassmorphic dashboard, health probes, metrics, goroutine dumps, and DLQ management.
- 📊 **[Custom Health Indicators & Telemetry](docs/tutorials/24_custom_actuator_health_and_telemetry.md)**:
  `web.RegisterHealthCheck`, external ping health indicators, and Kubernetes probe privacy.
- 📝 **[Structured Logging & Distributed Tracing](docs/tutorials/15_structured_logging_and_tracing.md)**:
  `log/slog` structured logging, context propagation (`request_id`, `trace_id`), and JSON formats.
- 🧪 **[Unit & Integration Testing Guide](docs/tutorials/16_integration_and_unit_testing.md)**:
  `SprinGoTestContext`, fluent HTTP client, automatic DB rollback, and bean mocking.

---

## 🔐 Security & Safety

- **Actuator Health Privacy**: Public unauthenticated `GET /actuator/health` returns minimal status `{"status": "UP"}` for load balancers and Kubernetes probes without exposing database connection pool metrics, goroutines, or system topology. Authenticated requests (and the embedded Glassmorphic Admin Dashboard via Basic Auth) receive full detailed component metrics.
- **Production & Staging Hardening**: Profile validation (`prod`, `production`, `staging`, `stage`) automatically enforces 256-bit JWT secret entropy, explicit non-empty Actuator Basic Auth passwords, and restricts algorithms to supported standard types (`HS256`, `RS256`).
- **Boundary-Safe Path Matching**: Segment-boundary route checking (`/actuator` and `/actuator/*`) prevents sibling paths from accidentally bypassing JWT security or entering Basic Auth.
- **Modern OWASP Security Headers**: `SecurityHeadersMiddleware` automatically enforces `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy: camera=(), microphone=(), geolocation=(), payment=()`, `X-Permitted-Cross-Domain-Policies: none`, `X-Frame-Options: DENY`, `X-Content-Type-Options: nosniff`, and conditional `HSTS`.

---

## 🤝 Contributors

Thank you to all the people who contribute to **SprinGo Framework**!

<a href="https://github.com/NeftaliAcosta/springo/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=NeftaliAcosta/springo" alt="Contributors" />
</a>

---

## 📄 License

SprinGo Framework is open-source software licensed under the **[MIT License](LICENSE)**.
