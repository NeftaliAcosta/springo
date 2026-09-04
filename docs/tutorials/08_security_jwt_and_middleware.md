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

## 3. Extracting the Authenticated Principal & Roles in Services

When a valid Bearer token is provided, SprinGo automatically populates `context.Context`:

**Suggested File Path**: `internal/application/service/order_service.go`
```go
package service

import (
    "context"
    "fmt"

    "github.com/NeftaliAcosta/springo/framework/security"
)

func (s *OrderService) GetMyOrders(ctx context.Context) ([]Order, error) {
    // Extract authenticated principal
    username := security.GetUser(ctx)
    if username == "" {
        return nil, fmt.Errorf("unauthorized")
    }

    // Extract roles (prefixed with ROLE_)
    roles := security.GetRoles(ctx)

    // Extract raw bearer token for downstream API calls
    token := security.GetBearerToken(ctx)

    // Extract full claims map if needed
    claims := security.GetClaims(ctx)

    return s.repo.FindByUser(ctx, username)
}
```

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

## 5. Custom Middleware Integration

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
