# 🌐 Step-by-Step Guide: API Versioning, Route Groups & Response Capture Middleware

This tutorial demonstrates how to organize enterprise APIs with clean URL versioning (`/v1`, `/v2`), scoped middleware
chains per route group, and post-process outgoing HTTP responses (e.g. payload encryption, compression, audit logging,
HMAC signing) using `web.RouteGroup` and `web.ResponseCaptureMiddleware`.

---

## 1. Overview & Motivation

In enterprise architectures, microservices frequently need to:
1. Support multiple active API versions (`/v1` for legacy mobile apps, `/v2` for modern web clients).
2. Apply distinct middleware pipelines per version (e.g. older authentication headers for v1 vs modern JWT for v2).
3. Inspect and transform outgoing HTTP responses after controllers return (e.g. adding cryptographic signatures,
   masking sensitive fields, or dynamic payload compression).

SprinGo provides two purpose-built primitives:
- **`web.RouteGroup(subPath, middlewares, register)`**: Registers endpoints under a sub-path with scoped middlewares.
- **`web.ResponseCaptureMiddleware` & `web.CapturedResponse`**: Buffers HTTP status, headers, and body in memory,
  allowing downstream or wrapping middlewares to inspect and mutate the response before it is written to the client.

---

## 2. API Versioning with `web.RouteGroup`

Controllers register versioned sub-routes cleanly using `web.RouteGroup`:

```go
package rest

import (
    "context"
    "net/http"

    "github.com/NeftaliAcosta/springo/framework/ioc"
    "github.com/NeftaliAcosta/springo/framework/web"
    "github.com/go-chi/chi/v5"
)

type ProductController struct {
    // Port dependencies injected via IoC
}

func init() {
    ioc.RegisterBean("productController", &ProductController{})

    // Group 1: /api/v1/products (with legacy headers middleware)
    web.RouteGroup("/v1/products", []func(http.Handler) http.Handler{legacyDeprecationWarningMiddleware}, func(r chi.Router) {
        c, _ := ioc.Get[ProductController]("productController")
        r.Get("/", web.Dispatch(c.listProductsV1))
        r.Get("/{id}", web.Dispatch(c.getProductByIDV1))
    })

    // Group 2: /api/v2/products (with response capture & payload signing middleware)
    web.RouteGroup("/v2/products", []func(http.Handler) http.Handler{
        web.ResponseCaptureMiddleware,
        payloadSignatureMiddleware,
    }, func(r chi.Router) {
        c, _ := ioc.Get[ProductController]("productController")
        r.Get("/", web.Dispatch(c.listProductsV2))
        r.Post("/", web.Dispatch(c.createProductV2))
    })
}
```

Routes in `/v1/products` will only execute `legacyDeprecationWarningMiddleware`, while routes in `/v2/products` will
execute `web.ResponseCaptureMiddleware` and `payloadSignatureMiddleware`.

---

## 3. Response Capture & Post-Processing Middleware

When you need to inspect or transform the outgoing response (such as adding an HMAC signature or logging response data):

```go
package middleware

import (
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "net/http"

    "github.com/NeftaliAcosta/springo/framework/web"
)

// SignatureMiddleware computes an HMAC-SHA256 signature of the response body and attaches it as a header.
func SignatureMiddleware(secretKey string) func(http.Handler) http.Handler {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            // 1. Let the downstream handlers and controllers execute
            next.ServeHTTP(w, r)

            // 2. Retrieve the buffered response from context
            captured, ok := web.GetCapturedResponse(r.Context())
            if !ok || len(captured.Body) == 0 {
                return
            }

            // 3. Compute cryptographic signature over the buffered body
            h := hmac.New(sha256.New, []byte(secretKey))
            h.Write(captured.Body)
            signature := hex.EncodeToString(h.Sum(nil))

            // 4. Attach signature header to outgoing response
            captured.Headers.Set("X-Payload-Signature", signature)
        })
    }
}
```

---

## 4. Modifying Response Payload and Status Code

Downstream middlewares can also rewrite the response payload (for example, payload compression or field redaction):

```go
func ResponseMaskingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        next.ServeHTTP(w, r)

        captured, ok := web.GetCapturedResponse(r.Context())
        if !ok {
            return
        }

        // Check and transform payload if necessary
        if captured.StatusCode == http.StatusOK {
            captured.Body = sanitizeSensitiveFields(captured.Body)
            captured.Headers.Set("X-Data-Sanitized", "true")
        }
    })
}
```

---

## 5. Streaming and WebSocket Compatibility

`web.ResponseCaptureMiddleware` implements `http.Flusher` and `http.Hijacker` interfaces. When handling Server-Sent
Events (SSE) or WebSockets, connection hijacking and stream flushing are automatically delegated to the underlying
network connection without buffer interference.
