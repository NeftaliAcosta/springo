# 🏗️ Step-by-Step Guide: Flattened Hexagonal Architecture

This tutorial explains the architectural design, folder hierarchy, and layer separation rules enforced by SprinGo.

---

## 1. Overview

SprinGo enforces a **Flattened Hexagonal Architecture** (Ports and Adapters) with a clean separation of concerns:
- **Domain (Core)**: Entities and domain error sentinels. Zero dependencies on frameworks or databases.
- **Application (Use Cases)**: Inbound/Outbound ports and business logic orchestration services.
- **Infrastructure (Adapters)**: REST controllers, GORM persistence repositories, JWT middleware, and external clients.

---

## 2. Directory Structure

```text
my-service/
├── cmd/app/main.go                           # Application bootstrapper
├── internal/
│   ├── domain/                               # 1. CORE DOMAIN LAYER
│   │   ├── model/                            # Pure domain entities (e.g. User, Invoice)
│   │   └── errors/                           # Sentinel errors (e.g. ErrUserNotFound)
│   ├── application/                          # 2. USE CASE LAYER
│   │   ├── port/input/                       # Inbound interfaces (Use Cases)
│   │   ├── port/output/                      # Outbound interfaces (Repositories, Gateways)
│   │   └── service/                          # Application service implementations
│   └── infrastructure/                       # 3. ADAPTERS LAYER
│       ├── config/                           # YAML property bindings & lifecycle hooks
│       ├── dtos/request/                     # Inbound request DTOs with validation tags
│       ├── dtos/response/                    # Outbound response DTOs
│       ├── input/rest/                       # Chi HTTP REST controllers (web.Dispatch)
│       └── output/persistence/               # GORM repository adapters
└── resources/
    ├── application.yaml                      # Configuration files
    └── db/migration/                         # Database schema migrations
```

---

## 3. Layer Implementation Examples

### 3.1 Domain Model
**Suggested File Path**: `internal/domain/model/user.go`
```go
package model

import "time"

type User struct {
    ID        uint
    Email     string
    FullName  string
    CreatedAt time.Time
}
```

### 3.2 Inbound Port (Use Case Contract)
**Suggested File Path**: `internal/application/port/input/user_usecase.go`
```go
package input

import (
    "context"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
)

type UserUseCase interface {
    GetByID(ctx context.Context, id uint) (*model.User, error)
}
```

### 3.3 Application Service
**Suggested File Path**: `internal/application/service/user_service.go`
```go
package service

import (
    "context"

    "github.com/NeftaliAcosta/springo/demo-api/internal/application/port/output"
    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
)

type UserService struct {
    repo output.UserRepository `spring:"userRepository"`
}

func (s *UserService) GetByID(ctx context.Context, id uint) (*model.User, error) {
    return s.repo.FindByID(ctx, id)
}
```

### 3.4 REST Controller (Inbound Adapter)
**Suggested File Path**: `internal/infrastructure/input/rest/user_controller.go`
```go
package rest

import (
    "context"

    "github.com/NeftaliAcosta/springo/demo-api/internal/application/port/input"
    "github.com/NeftaliAcosta/springo/framework/ioc"
    "github.com/NeftaliAcosta/springo/framework/web"
    "github.com/go-chi/chi/v5"
)

type UserController struct {
    useCase input.UserUseCase `spring:"userService"`
}

func init() {
    ioc.RegisterBean("userController", &UserController{})

    web.RegisterRoutes(func(r chi.Router) {
        c, _ := ioc.Get[UserController]("userController")
        r.Get("/users/{id}", web.Dispatch(c.getUserByID))
    })
}

func (c *UserController) getUserByID(ctx context.Context, req struct {
    ID uint `path:"id" validate:"required,gt=0"`
}) (any, error) {
    return c.useCase.GetByID(ctx, req.ID)
}
```
