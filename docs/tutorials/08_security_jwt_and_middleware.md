# 🔐 Step-by-Step Guide: OAuth2 Resource Server, JWT Security & Middleware

This tutorial explains how to configure authentication with OAuth2 / OIDC (Keycloak, Auth0, Okta, Azure AD),
OWASP security headers, and security context extraction in SprinGo.

---

## 1. Overview

SprinGo includes an enterprise-grade security subsystem:
- **OAuth2 Resource Server**: First-class support for OIDC providers (`RS256` via JWKS or symmetric `HS256`).
- **Configurable Claims & Roles**: Extraction of principals (`principal-claim`) and roles (`authorities-claim`).
- **Pluggable Token Validators**: Register custom claim validators with `security.RegisterTokenValidator()`.
- **Security Context API**: Type-safe extraction of user, roles, raw bearer token, and claims from `context.Context`.
- **Public & Protected Paths**: Configurable route whitelist with Ant-style wildcards (e.g. `/swagger/**`).
- **OWASP Security Headers**: Automatic enforcement of `Referrer-Policy`, `X-Frame-Options`, and `HSTS`.

---

## 2. Configuring OAuth2 Resource Server (Keycloak / OIDC)

**Suggested File Path**: `resources/application.yaml`
```yaml
security:
  oauth2:
    resourceserver:
      enabled: true
      issuer-uri: "https://sso.example.com/realms/production"
      jwks-uri: "https://sso.example.com/realms/production/protocol/openid-connect/certs"
      algorithm: RS256
      principal-claim: preferred_username
      resource-id: my-microservice-client
      authority-prefix: "ROLE_"
      audience:
        - my-microservice-client
      public-paths:
        - "/actuator/health"
        - "/swagger/**"
        - "/api/v1/public/**"
```

For symmetric HS256 setups (self-contained tokens or development):
```yaml
security:
  oauth2:
    resourceserver:
      enabled: true
      algorithm: HS256
      secret: ${JWT_SECRET:my-very-long-secret-key-that-exceeds-32-bytes!}
      public-paths:
        - "/swagger/**"
```

---

## 3. Security Context API & Downstream Token Propagation

When a valid Bearer token is provided, SprinGo automatically populates `context.Context` with strongly-typed, immutable accessors:

**Suggested File Path**: `internal/application/service/order_service.go`
```go
package service

import (
    "context"
    "fmt"
    "net/http"

    "github.com/NeftaliAcosta/springo/framework/security"
)

func (s *OrderService) GetMyOrders(ctx context.Context) ([]Order, error) {
    // 1. Extract authenticated principal
    username := security.GetUser(ctx)
    if username == "" {
        return nil, fmt.Errorf("unauthorized")
    }

    // 2. Role verification (zero-allocation helper)
    if !security.HasRole(ctx, "ROLE_USER") {
        return nil, fmt.Errorf("forbidden: user role required")
    }

    // 3. Extract specific typed claim without cloning the full map
    tenantID, ok := security.GetClaim[string](ctx, "tenant_id")
    if !ok {
        return nil, fmt.Errorf("missing tenant_id claim")
    }

    // 4. Downstream Bearer token propagation to external microservices
    token := security.GetBearerToken(ctx)
    req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://inventory-service/api/stock", nil)
    req.Header.Set("Authorization", "Bearer "+token)

    return s.repo.FindByUserAndTenant(ctx, username, tenantID)
}
```

### Available Security Context Helpers:
- `security.GetUser(ctx)`: Returns principal username or empty string.
- `security.GetRoles(ctx)`: Returns defensive copy of granted authorities slice.
- `security.HasRole(ctx, role)`: Verifies role membership with zero allocations.
- `security.HasAnyRole(ctx, roles...)`: Verifies if context has any of the listed roles.
- `security.GetClaims(ctx)`: Returns a defensive cloned copy of JWT claims `map[string]any`.
- `security.GetClaim[T](ctx, key)`: Zero-allocation typed claim accessor (`(T, bool)`).
- `security.GetBearerToken(ctx)`: Returns raw token string for downstream HTTP calls.
- `security.GetUserInfo(ctx)`: Returns consolidated `*UserInfo` struct.

---

## 4. Custom Token Validators

You can register pluggable token validators executed during request authentication:

**Suggested File Path**: `internal/infrastructure/security/tenant_validator.go`
```go
package security

import (
    "context"
    "fmt"

    "github.com/NeftaliAcosta/springo/framework/security"
    "github.com/golang-jwt/jwt/v5"
)

type TenantValidator struct{}

func (v *TenantValidator) Validate(ctx context.Context, claims jwt.MapClaims) error {
    tenantID, ok := claims["tenant_id"].(string)
    if !ok || tenantID == "" {
        return fmt.Errorf("missing tenant_id in token")
    }
    return nil
}

func init() {
    security.RegisterTokenValidator(&TenantValidator{})
}
```

---

## 5. Web Server Security Pipeline & Stateless REST API Mode (`server.security`)

SprinGo provides a modular, zero-config HTTP security pipeline configured under `server.security`:

```yaml
server:
  security:
    csrf-enabled: false              # Default: false (stateless REST APIs / OAuth2 Bearer tokens)
    security-headers-enabled: true   # Default: true (OWASP security headers)
    cors-enabled: true               # Default: true (Cross-Origin Resource Sharing)
    headers:
      content-type-options: "nosniff"
      frame-options: "DENY"          # DENY | SAMEORIGIN
      xss-protection: "0"
      referrer-policy: "strict-origin-when-cross-origin"
      permissions-policy: "camera=(), microphone=(), geolocation=(), payment=()"
      cross-domain-policies: "none"
      content-security-policy: "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:;"
      hsts:
        max-age: 31536000            # Seconds (default: 1 year)
        include-subdomains: true     # Include subdomains (default: true)
        preload: false               # Preload header flag (default: false)
  cors:
    allowed-origins:
      - "${FRONTEND_URL:http://localhost:3000}"
    allowed-methods: ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"]
    allowed-headers: ["Authorization", "Content-Type", "X-Requested-With", "X-Trace-ID"]
    allow-credentials: true
    max-age: 3600
```

### When to Keep CSRF Disabled vs When to Enable It
- **Stateless REST APIs (Default):** APIs authenticating via `Authorization: Bearer <token>` (JWT / OAuth2) are immune to CSRF because browsers do not automatically attach Bearer headers on cross-site requests. SprinGo automatically disables CSRF protection when OAuth2 Resource Server is active or by default.
- **Cookie-Based Web Applications:** If your application relies on ambient browser cookies or session cookies for authentication, enable CSRF explicitly:
  ```yaml
  server:
    security:
      csrf-enabled: true
  spring:
    security:
      csrf:
        cookie-name: "XSRF-TOKEN"
        header-name: "X-XSRF-TOKEN"
  ```

---

## 6. Custom Middleware Integration

Register global middlewares during bootstrap:

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
        // Custom rate limiting logic
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
