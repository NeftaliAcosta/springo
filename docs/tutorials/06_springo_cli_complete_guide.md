# 🛠️ Complete Reference Guide: SprinGo CLI (`springo`)

This document is the comprehensive manual for all commands, flags, and code generators provided by the `springo` CLI.

---

## 1. Installation

Install or update the `springo` binary directly using the Go toolchain:

```bash
# Install specific Release Candidate or stable release
go install github.com/NeftaliAcosta/springo/cmd/springo@v1.0.0-rc19

# Verify installation
springo version
```

---

## 2. Command Overview

| Command | Purpose |
| :--- | :--- |
| `springo new <name>` | Scaffold a new microservice with Flattened Hexagonal Architecture. |
| `springo run` | Start the local development server with fast incremental builds. |
| `springo make <type> <name>` | Generate clean hexagonal architectural components (model, dto, service, etc.). |
| `springo migrate` | Run pending database migrations with ShedLock synchronization. |
| `springo migrate status` | Inspect applied and pending database migration batches. |
| `springo migrate rollback` | Roll back recent migration batches. |
| `springo routes` | List all discovered HTTP routes, methods, and registered handlers. |
| `springo swagger` | Generate Swagger/OpenAPI 2.0 specifications from code annotations. |
| `springo version` | Print current CLI and framework release version. |

---

## 3. Scaffolding Projects (`springo new`)

Creates a complete, ready-to-run microservice:

```bash
# Scaffold a new billing service
springo new billing-api

# Navigate into project directory
cd billing-api

# Launch immediate development server
go run cmd/app/main.go
```

### Generated Project Structure
```text
billing-api/
├── cmd/app/main.go                           # Application entrypoint
├── internal/
│   ├── application/
│   │   ├── port/input/                       # Inbound use cases
│   │   ├── port/output/                      # Outbound repository/gateway ports
│   │   └── service/                          # Application business logic
│   ├── domain/
│   │   ├── model/                            # Core domain entities
│   │   └── errors/                           # Exported domain error sentinels
│   └── infrastructure/
│       ├── config/                           # YAML property bindings & lifecycle
│       ├── dtos/                             # Request and response DTOs
│       ├── input/rest/                       # Chi HTTP controllers
│       └── output/persistence/               # GORM database adapters
├── resources/
│   ├── application.yaml                      # Default configuration
│   └── db/migration/                         # SQL / Go migration files
├── go.mod
└── README.md
```

---

## 4. Code Generation (`springo make`)

Generate consistent, clean-architecture components instantly:

### 4.1 Models (`springo make model`)
```bash
springo make model Order
# Creates: internal/domain/model/order.go
```

### 4.2 DTOs (`springo make dto`)
```bash
springo make dto Order
# Creates:
#   internal/infrastructure/dtos/request/create_order_request.go
#   internal/infrastructure/dtos/response/order_response.go
```

### 4.3 Repositories (`springo make repository`)
```bash
springo make repository Order
# Creates:
#   internal/application/port/output/order_repository.go
#   internal/infrastructure/output/persistence/order_repository_adapter.go
```

### 4.4 Services (`springo make service`)
```bash
springo make service Order
# Creates:
#   internal/application/port/input/order_use_case.go
#   internal/application/service/order_service.go
```

### 4.5 Controllers (`springo make controller`)
```bash
springo make controller Order
# Creates: internal/infrastructure/input/rest/order_controller.go
```

### 4.6 Migrations (`springo make migration`)
```bash
springo make migration CreateOrdersTable
# Creates: resources/db/migration/V1.0__create_orders_table.sql
```

---

## 5. Database Migrations (`springo migrate`)

Run, inspect, and revert database migrations from the terminal:

```bash
# Execute all pending migrations
springo migrate

# Check migration status and batch history
springo migrate status

# Roll back the most recent batch
springo migrate rollback

# Roll back specific number of batches
springo migrate rollback --steps=2
```

---

## 6. Route Discovery & OpenAPI Documentation

```bash
# Print registered endpoints in table format
springo routes

# Generate OpenAPI/Swagger documentation
springo swagger

# Generate silently without console noise
springo swagger --quiet

# Specify custom application entrypoint
springo swagger --main ./cmd/custom/main.go
```
