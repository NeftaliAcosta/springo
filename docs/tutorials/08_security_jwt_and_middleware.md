# 🔐 Step-by-Step Guide: JWT Security, OWASP Headers & Middleware

This tutorial explains how to configure authentication, authorization, OWASP security headers, and custom HTTP
middlewares in SprinGo.

---

## 1. Overview

SprinGo includes an enterprise-grade security subsystem:
- **JWT Provider**: Supports symmetric (`HS256`) and asymmetric keys (`RS256` for Keycloak, Auth0, Okta JWKS).
- **Public & Protected Paths**: Configurable route whitelist in `application.yaml`.
- **OWASP Security Headers**: Automatic enforcement of `Referrer-Policy`, `X-Frame-Options`, and `HSTS`.
- **CSRF Protection**: Token-based double-submit cookie validation for browser clients.
- **Middleware Pipeline**: Idiomatic middleware registration matching `func(http.Handler) http.Handler`.

---

## 2. Configuring JWT Security

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  security:
    jwt:
      enabled: true
      secret: ${JWT_SECRET:a-very-secure-32-character-secret-key!}
      algorithm: HS256
      issuer: "https://auth.example.com"
      expiration: 24h
      public-paths:
        - "/api/v1/auth/login"
        - "/api/v1/auth/register"
        - "/actuator/health"
        - "/swagger/*"
```

---

## 3. Extracting the Authenticated User in Handlers

When a valid Bearer token is provided, SprinGo populates user claims in `context.Context`:

**Suggested File Path**: `internal/application/service/order_service.go`
```go
package service

import (
    "context"
    "fmt"

    "github.com/NeftaliAcosta/springo/framework/security"
)

func (s *OrderService) GetMyOrders(ctx context.Context) ([]Order, error) {
    // Extract authenticated claims
    claims := security.GetClaims(ctx)
    if claims == nil {
        return nil, fmt.Errorf("unauthorized")
    }

    userID := claims.Subject
    email := claims.Email
    roles := claims.Roles

    return s.repo.FindByUserID(ctx, userID)
}
```

---

## 4. Custom Middleware Integration

Register global middlewares in `cmd/app/main.go`:

**Suggested File Path**: `cmd/app/main.go`
```go
package main

import (
    "net/http"

    "github.com/NeftaliAcosta/springo/framework"
    "github.com/NeftaliAcosta/springo/framework/web"
)

func RateLimitMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Custom rate limiting logic here
        next.ServeHTTP(w, r)
    })
}

func main() {
    framework.Bootstrap(framework.Options{
        Middlewares: []func(http.Handler) http.Handler{
            web.SecurityHeadersMiddleware, // OWASP Headers
            RateLimitMiddleware,           // Custom Limiter
        },
    }).Start()
}
```
