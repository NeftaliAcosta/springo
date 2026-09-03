# 🛡️ Step-by-Step Guide: Advanced DTO Validation & Validation Groups

This tutorial explains how to use Spring-style validation groups (`OnCreate`, `OnUpdate`), custom validation tags,
and nested struct validation in SprinGo.

---

## 1. Overview

SprinGo provides a rich validation pipeline integrated with `web.Dispatch`:
- **Validation Groups**: Conditionally validate fields during creation vs update via `web.WithValidationGroup`.
- **Predefined Groups**: Standard `web.OnCreate{}` and `web.OnUpdate{}` markers.
- **Declarative Validator Tags**: Full support for Go Playground validator rules
  (`required`, `email`, `gt`, `uuid`, etc.).
- **Automatic Problem Details**: Formats validation errors into standard RFC 7807 responses.

---

## 2. Defining DTOs with Validation Groups

**Suggested File Path**: `internal/infrastructure/dtos/request/user_dto.go`
```go
package request

type UserDTO struct {
    ID       uint   `json:"id" validate:"required,gt=0"`         // Required only on Update
    Email    string `json:"email" validate:"required,email"`     // Always required
    Password string `json:"password" validate:"required,min=8"`  // Required on Create
    FullName string `json:"full_name" validate:"required,min=3"`
}
```

---

## 3. Applying Validation Groups in Routes

**Suggested File Path**: `internal/infrastructure/input/rest/user_controller.go`
```go
package rest

import (
    "github.com/NeftaliAcosta/springo/framework/web"
    "github.com/go-chi/chi/v5"
)

func RegisterUserRoutes(r chi.Router, c *UserController) {
    r.Route("/users", func(r chi.Router) {
        // Enforce OnCreate validation rules on POST
        r.Post("/", web.Dispatch(
            c.createUser,
            web.WithValidationGroup(web.OnCreate{}),
        ))

        // Enforce OnUpdate validation rules on PUT
        r.Put("/{id}", web.Dispatch(
            c.updateUser,
            web.WithValidationGroup(web.OnUpdate{}),
        ))
    })
}
```

---

## 4. Custom Validator Rules

**Suggested File Path**: `internal/infrastructure/config/validator_config.go`
```go
package config

import (
    "github.com/NeftaliAcosta/springo/framework/web"
    "github.com/go-playground/validator/v10"
)

func init() {
    web.RegisterCustomValidation("alphanumeric_slug", func(fl validator.FieldLevel) bool {
        slug := fl.Field().String()
        return isSlugValid(slug)
    })
}
```
