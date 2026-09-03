# 🌐 Step-by-Step Guide: REST Web Routing, Security Privileges & Swagger

This tutorial explains how to build high-performance HTTP REST endpoints, enforce role-based access control,
bind request parameters, and generate OpenAPI / Swagger documentation using SprinGo.

---

## 1. Overview

SprinGo's web layer integrates with the high-performance **Chi router**:
- **Automatic Dispatching**: `web.Dispatch` maps controller methods to Chi handlers without HTTP boilerplate.
- **Role-Based Privilege Validation**: Restrict endpoints to specific roles directly in the router definition:
  `r.Post("/", web.Dispatch(c.create, "ADMIN"))`.
- **Transactional Endpoints (`web.DispatchTx`)**: Automatically wraps entire request execution in a DB transaction.
- **Declarative DTO Binding**: Binds JSON bodies, URL path variables (`path:"id"`), query params (`query:"page"`),
  and multipart files (`form:"file"`).
- **Validation Engine**: Built-in Go playground validator tags (`validate:"required,email,min=8"`).
- **OpenAPI / Swagger Generation**: Automatic API documentation generated via the `springo swagger` CLI tool.

---

## 2. Router Structure & Role-Based Privilege Validation

Register routes in `internal/infrastructure/input/rest/` using `web.RegisterRoutes`:

```go
package rest

import (
    "context"
    "net/http"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/port/in"
    "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
    "github.com/NeftaliAcosta/springo/framework/ioc"
    "github.com/NeftaliAcosta/springo/framework/web"
    "github.com/go-chi/chi/v5"
)

type UserController struct {
    useCase in.UserUseCase `spring:"userUseCase"`
}

func init() {
    ioc.RegisterBean("userController", &UserController{})

    web.RegisterRoutes(func(r chi.Router) {
        c, _ := ioc.Get[UserController]("userController")

        r.Route("/users", func(r chi.Router) {
            // Public or standard authenticated endpoint
            r.Get("/{id}", web.Dispatch(c.getUserByID))

            // Restricted to users with 'ADMIN' role
            r.Post("/", web.Dispatch(c.createUser, "ADMIN"))

            // Restricted to users with either 'ADMIN' OR 'MANAGER' roles
            r.Put("/{id}", web.Dispatch(c.updateUser, "ADMIN", "MANAGER"))

            // Restricted to 'ADMIN' with functional option and custom 200 OK status
            r.Delete("/{id}", web.Dispatch(
                c.deleteUser,
                web.WithRoles("ADMIN"),
                web.WithSuccessStatus(http.StatusOK),
            ))
        })
    })
}
```

---

## 3. Transactional Endpoints with `web.DispatchTx`

When an endpoint needs to execute completely inside an isolated database transaction with automatic rollback
on error:

```go
// Automatically begins a DB transaction, binds DTO, calls controller, and commits/rolls back
r.Post("/transfer", web.DispatchTx(c.transferFunds, "ADMIN"))
```

---

## 4. Swagger / OpenAPI Code Annotations

SprinGo uses standard declarative annotations above controller methods to generate OpenAPI documentation:

```go
// @Summary Create a new user
// @Description Creates a user account with assigned role and returns the created record
// @Tags Users
// @Accept json
// @Produce json
// @Param tenant_id path string true "Tenant UUID"
// @Param request body request.CreateUserRequest true "User Registration Payload"
// @Success 201 {object} response.UserResponse
// @Failure 400 {object} errors.ProblemDetails "Validation Error"
// @Failure 401 {object} errors.ProblemDetails "Unauthorized"
// @Failure 403 {object} errors.ProblemDetails "Forbidden (Insufficient Privileges)"
// @Router /api/v1/users [post]
// @Security BearerAuth
func (c *UserController) createUser(
    ctx context.Context,
    req request.CreateUserRequest,
) (any, error) {
    return c.useCase.CreateUser(ctx, req)
}
```

---

## 5. Regenerating Swagger Documentation (`springo swagger`)

Whenever you create new endpoints, modify DTO structs, or update documentation annotations, regenerate the
Swagger specification from the project root:

```bash
# Regenerate Swagger / OpenAPI specs
springo swagger

# Regenerate silently in automated CI/CD scripts
springo swagger --quiet

# Specify custom main entrypoint if located elsewhere
springo swagger --main ./cmd/custom/main.go
```

The interactive Swagger UI is immediately viewable locally at:
`http://localhost:8080/swagger/index.html`

---

## 6. Multipart File Uploads

```go
type UploadAvatarRequest struct {
    UserID uint               `path:"id" validate:"required"`
    Avatar *web.MultipartFile `form:"avatar" validate:"required"`
}

func (c *UserController) uploadAvatar(
    ctx context.Context,
    req UploadAvatarRequest,
) (any, error) {
    file, err := req.Avatar.Open()
    if err != nil {
        return nil, err
    }
    defer file.Close()

    return map[string]any{
        "filename": req.Avatar.Filename,
        "size":     req.Avatar.Size,
    }, nil
}
```
