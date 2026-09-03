# 🧪 Step-by-Step Guide: Unit & Integration Testing in SprinGo

This tutorial explains how to write fast, isolated unit tests and end-to-end integration tests using SprinGo's
native `framework/test` testing toolkit.

---

## 1. Overview

SprinGo provides a specialized testing environment designed for Spring Boot developers:
- **`NewSprinGoTestContext`**: Bootstraps the entire application stack in isolated test mode.
- **Automatic Transaction Rollback**: Starts a database transaction at test startup and rolls back on teardown so tests
  never pollute the database.
- **Fluent HTTP Test Client**: Chainable request builder (`Get`, `Post`, `WithHeader`, `WithJSON`, `Execute`).
- **Declarative Assertions**: Fluent response status and body assertions (`ExpectStatus`, `ExpectBodyContains`).
- **Bean Mocking (`ReplaceBean`)**: Override specific IoC beans with mock implementations per test.

---

## 2. End-to-End API Testing with `SprinGoTestContext`

**Suggested File Path**: `internal/infrastructure/input/rest/user_controller_test.go`
```go
package rest_test

import (
    "testing"

    "github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
    "github.com/NeftaliAcosta/springo/framework/test"
)

func TestUserRegistration_E2E(t *testing.T) {
    // 1. Initialize isolated test context with automatic DB rollback
    app := test.NewSprinGoTestContext(t)
    defer app.TearDown()

    // 2. Prepare request payload
    payload := request.CreateUserRequestDTO{
        Email:    "jane.doe@example.com",
        FullName: "Jane Doe",
        Age:      28,
        Role:     "USER",
    }

    // 3. Execute fluent HTTP POST request and assert status
    app.Client.Post("/api/v1/users").
        WithHeader("X-Tenant-ID", "tenant-100").
        WithJSON(payload).
        Execute().
        ExpectStatus(201).
        ExpectBodyContains("jane.doe@example.com")
}
```

---

## 3. Mocking Dependencies with `ReplaceBean`

**Suggested File Path**: `internal/application/service/checkout_service_test.go`
```go
package service_test

import (
    "testing"

    "github.com/NeftaliAcosta/springo/framework/test"
    "github.com/stretchr/testify/mock"
)

type MockPaymentService struct {
    mock.Mock
}

func (m *MockPaymentService) Process(amount float64) error {
    return m.Called(amount).Error(0)
}

func TestCheckout_WithMockedGateway(t *testing.T) {
    app := test.NewSprinGoTestContext(t)
    defer app.TearDown()

    mockPayment := new(MockPaymentService)
    mockPayment.On("Process", 99.99).Return(nil)

    // Replace the real bean in the IoC container with the mock
    app.ReplaceBean("paymentService", mockPayment)

    app.Client.Post("/api/v1/checkout").
        WithJSON(map[string]any{"amount": 99.99}).
        Execute().
        ExpectStatus(200)

    mockPayment.AssertExpectations(t)
}
```

---

## 4. Running Tests

```bash
# Run all tests with race condition detector
go test -race -count=1 ./...
```
