# SprinGo Framework 🚀

SprinGo is a high-performance, opinionated enterprise framework for Go, specifically designed to provide a familiar environment for **Spring Boot (Java)** developers. It follows a clean architecture that strictly separates the framework's engine (`framework/`) from application code.

> **Release status:** `v1.0.0-rc1` is a Release Candidate. Validate it in a staging environment before adopting it for production workloads; public APIs may still receive release-blocking corrections before `v1.0.0`.

### Requirements

- Go `1.26.5` or newer compatible version.
- Git, when using the CLI's automatic repository initialization.

## 🏗️ Framework & Repo Architecture

```text
springo/
├── framework/                 # 🛠️ THE KERNEL (SprinGo Engine Library)
│   ├── app.go
│   ├── runner.go
│   ├── cache/
│   ├── config/
│   ├── data/
│   ├── database/
│   ├── errors/
│   ├── event/
│   ├── ioc/
│   ├── logging/
│   ├── scheduler/
│   ├── security/
│   ├── test/
│   └── web/
├── cmd/
│   └── cli/                   # 🛠️ SprinGo CLI (v1.0.0-rc1)
│       ├── main.go
│       └── cmd/               # Code generators & scaffolding templates
├── demo-api/                  # 🚀 Reference Application (Separated Demo API)
│   ├── cmd/app/
│   ├── internal/
│   └── resources/
├── LICENSE                    # MIT License
└── README.md
```

## 🚀 Key Features

- **SprinGo CLI & Code Generators**: Command line tool (`springo`) for scaffolding hexagonal architecture projects (`springo new`), database migrations, and generating components (`springo make model|dto|repository|service|controller|migration`).
- **Dynamic Actuator Health**: Intelligent monitoring that automatically discovers all database connections and collects system metrics (uptime, memory, platform).
- **One-Line Bootstrap**: Start your entire engine (Config, IoC, Router, Middleware) with a single call to `framework.Bootstrap()`.
- **Library-Style Kernel**: The framework logic lives in `/framework` at the root, acting as a standalone, decoupled library.
- **Magic Dispatcher & @Valid**: Automatic JSON decoding and **DTO Validation** (@Valid equivalent) using tags.
- **Global Error Handler**: Centralized exception management (style @ControllerAdvice).
- **Standardized Responses**: All API outputs are automatically wrapped in a consistent JSON structure.
- **CORS Configurable**: Robust CORS management via `application.yaml` (origins, methods, headers, credentials).
- **JWT Security & Boundary Whitelisting**: Boundary-safe path whitelisting with JWT validation and fail-fast configuration checks on startup (Properties Validation).
- **Transactional Support & Propagation**: Declarative-style transactions with GORM supporting `REQUIRED` propagation to join nested scopes naturally.
- **Domain Event Bus & Scheduled Jobs**: Pub/Sub events with Outbox buffer/DLQ support and CRON scheduled background tasks.
- **Full Auto-Wiring & IoC**: Automatic dependency management through a centralized container.
- **Component Scanning**: Components register themselves automatically using Go's `init()` functions and `RegistrationHooks`.
- **Dynamic Port Discovery**: Automatically finds an open port starting from 8080.
- **Configuration Profiles**: Strict support for `application-{profile}.yaml` environments.

---

## 🛠️ SprinGo CLI (v1.0.0-rc1)

SprinGo includes an official Command Line Interface (`springo`) designed to accelerate application creation, code generation, migration execution, and route inspection.

### Installation

```bash
go install github.com/NeftaliAcosta/springo/cmd/cli@v1.0.0-rc1
# Or build locally from repository root:
go build -o springo cmd/cli/main.go
```

### 1. Scaffolding New Projects (`springo new`)

Generates a fully structured professional Hexagonal Architecture project:

```bash
springo new my-app
cd my-app
go mod tidy
go run cmd/app/main.go
```

Options:
- `--local`: Link local development framework module via `replace` directive.
- `--skip-git`: Skip initial `git init` setup.

### 2. Code Generators (`springo make`)

Generate hexagonal components instantly following SprinGo's enterprise naming conventions:

```bash
# Generate Domain Model
springo make model Product

# Generate Request & Response DTOs
springo make dto Product

# Generate Persistence Port, Entity (GORM) & Repository Adapter
springo make repository Product

# Generate Use Case Port & Application Service implementation
springo make service Product

# Generate REST Controller (Chi router integration)
springo make controller Product

# Generate Database Migration file
springo make migration CreateProductsTable
```

### 3. Database Migrations (`springo migrate`)

Execute Flyway-style SQL and Go migrations with automatic distributed cluster locking (`ShedLock`):

```bash
# Apply pending migrations
springo migrate

# Check migration execution status
springo migrate status

# Roll back the last N batches (default 1)
springo migrate rollback --steps=1

# Reset all applied migrations
springo migrate reset

# Refresh database (reset + re-run all migrations)
springo migrate refresh
```

### 4. Route Discovery (`springo routes`)

Inspect all registered REST endpoints of your application directly from the terminal without side-effects:

```bash
springo routes
```

---

## ⚙️ Configuration & Profiles

SprinGo strongly follows the **Convention over Configuration** principle. Configuration files are **completely optional**. If omitted, the framework falls back to sensible enterprise defaults without crashing.

SprinGo uses a profile system loaded via `SPRINGO_PROFILES_ACTIVE`:

| Profile | Variable | File Loaded |
| :--- | :--- | :--- |
| **Default** | (empty) | `resources/application.yaml` |
| **Dev** | `dev` | `resources/application-dev.yaml` |
| **Prod** | `prod` | `resources/application-prod.yaml` |

---

## 🔐 Security & Management

- **JWT Auth**: Supports HMAC-SHA256 (HS256) symmetric keys and RS256 RSA/JWKS asymmetric keys (Keycloak / Auth0 / Okta).
- **Actuator Console**: Web UI at `/actuator/dashboard` for monitoring health, metrics, goroutine dumps, beans, env properties, ShedLock jobs, and DLQ retries.
- **CSRF & Security Headers**: Built-in Double-Submit Cookie CSRF protection and OWASP security headers.

### Demo application safety

`demo-api/` is an executable reference application and a separate Go module. Its JWT secret and login credentials are intentionally public development examples. **Do not deploy the demo or reuse those credentials in production.** The framework rejects its known development JWT secret when the active profile is `prod` or `production`.

Because it is a nested module, validate it independently from the repository root:

```bash
(cd demo-api && go test -race ./...)
```

---

## 📄 License

SprinGo Framework is open-source software licensed under the **[MIT License](LICENSE)**.
