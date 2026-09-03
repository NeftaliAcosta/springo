# 🚨 Step-by-Step Guide: Standardized Error Handling & RFC 7807

This tutorial explains how to structure domain errors, handle validation failures, and format RFC 7807 Problem Details
responses in SprinGo.

---

## 1. Overview

SprinGo provides a standardized, enterprise-ready error subsystem:
- **RFC 7807 Compliance**: Automatically serializes errors to `application/problem+json`.
- **Domain Sentinels**: Map domain error constants to HTTP status codes cleanly.
- **Validation Formatter**: Translates DTO validation failures into field-level problem details.
- **Safe Internal Error Masking**: Masks database/internal errors in production to prevent data leakage.

---

## 2. Defining Domain Errors

**Suggested File Path**: `internal/domain/errors/product_errors.go`
```go
package errors

import (
    "errors"
    "net/http"

    frameworkErrors "github.com/NeftaliAcosta/springo/framework/errors"
)

var (
    ErrProductNotFound   = errors.New("product not found")
    ErrInsufficientStock = errors.New("insufficient product inventory")
)

func init() {
    // Map domain errors to HTTP statuses and user-friendly error codes
    frameworkErrors.RegisterHTTPMapping(ErrProductNotFound, http.StatusNotFound, "PRODUCT_NOT_FOUND")
    frameworkErrors.RegisterHTTPMapping(ErrInsufficientStock, http.StatusConflict, "INSUFFICIENT_STOCK")
}
```

---

## 3. Emitting Errors in Application Logic

**Suggested File Path**: `internal/application/service/product_service.go`
```go
package service

import (
    "context"

    "github.com/NeftaliAcosta/springo/demo-api/internal/domain/errors"
)

func (s *ProductService) DecreaseStock(ctx context.Context, id uint, quantity int) error {
    product, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return errors.ErrProductNotFound
    }

    if product.Stock < quantity {
        return errors.ErrInsufficientStock
    }

    product.Stock -= quantity
    return s.repo.Update(ctx, product)
}
```

---

## 4. RFC 7807 Response Output

When `ErrInsufficientStock` is returned, SprinGo automatically formats the response:

```http
HTTP/1.1 409 Conflict
Content-Type: application/problem+json

{
  "type": "https://api.example.com/errors/INSUFFICIENT_STOCK",
  "title": "Conflict",
  "status": 409,
  "detail": "insufficient product inventory",
  "instance": "/api/v1/products/42/stock",
  "code": "INSUFFICIENT_STOCK",
  "timestamp": "2026-09-02T21:30:00Z"
}
```
