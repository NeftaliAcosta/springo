# 🌐 Step-by-Step Guide: CORS & Origin Whitelist Configuration

This tutorial explains how to configure Cross-Origin Resource Sharing (CORS), exact origin whitelists, and wildcard
domain patterns in SprinGo.

---

## 1. Overview

SprinGo includes an automatic, zero-overhead CORS middleware registered by default under `server.cors`:
- **Exact Origin Whitelisting**: Specify exact allowed domains via `allowed-origins`.
- **Dynamic Wildcard Patterns**: Support subdomain patterns via `allowed-origin-patterns`
  (e.g. `https://*.example.com`).
- **Credential Safety Enforcement**: Automatically blocks insecure configurations (such as `allow-credentials: true`
  with `allowed-origins: ["*"]`).
- **Preflight (OPTIONS) Handling**: Automatically handles browser preflight requests with configurable `max-age`.
- **Header Exposure**: Specify custom headers exposed to frontend clients via `exposed-headers`.

---

## 2. Configuration

**Suggested File Path**: `resources/application.yaml` or `resources/application-prod.yaml`
```yaml
server:
  port: 8080
  cors:
    # Exact origin matches (supports environment variable fallbacks)
    allowed-origins:
      - "${FRONTEND_URL:http://localhost:3000}"
      - "http://localhost:5173"

    # Wildcard origin regex patterns
    allowed-origin-patterns:
      - "https://*.youbootcamps.com"
      - "http://*.youbootcamps.local"

    # Permitted HTTP methods
    allowed-methods:
      - "GET"
      - "POST"
      - "PUT"
      - "PATCH"
      - "DELETE"
      - "OPTIONS"

    # Allowed request headers
    allowed-headers:
      - "Authorization"
      - "Content-Type"
      - "X-Requested-With"
      - "X-Trace-ID"
      - "traceparent"

    # Response headers visible to JavaScript in the browser
    exposed-headers:
      - "X-Trace-ID"
      - "traceparent"

    # Support cookies, Authorization headers, and TLS client certificates
    allow-credentials: true

    # Preflight response cache duration in seconds
    max-age: 3600
```

---

## 3. Environment Variable Overrides

Override origins dynamically in container environments:

```bash
# Production Docker container run
docker run -p 8080:8080 \
  -e FRONTEND_URL="https://app.youbootcamps.com" \
  my-service:latest
```

---

## 4. Automatic Security Validations

SprinGo automatically validates CORS configuration at startup:
- **`allow-credentials: true` with `*` Origin**: Fails with error and blocks startup, preventing cross-origin credential
  theft.
- **Automatic `Vary: Origin` Header**: Automatically added when multiple origins or dynamic origin patterns are
  configured, preventing intermediate proxy/CDN cache poisoning.
