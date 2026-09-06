# 🚀 Step-by-Step Guide: Outbound HTTP Clients, IoC & Token Propagation

This tutorial demonstrates how to configure and register production-ready `http.Client` instances as IoC beans in
SprinGo, manage connection pooling, enforce request timeouts, and propagate security context (JWT tokens and tracing
headers) to downstream microservices.

---

## 1. Overview & Architecture

In distributed microservice architectures, making outbound HTTP calls requires:
1. **Connection Pooling & Reuse**: Custom `http.Transport` with configured `MaxIdleConns`, `MaxIdleConnsPerHost`, and
   `IdleConnTimeout` to prevent TCP socket exhaustion.
2. **Deterministic Timeouts**: Global and per-request deadlines to prevent hanging outbound calls from locking server goroutines.
3. **IoC Lifecycle Management**: Registering `*http.Client` as a singleton bean for seamless dependency injection.
4. **Context & Token Propagation**: Extracting incoming JWT bearer tokens (`security.GetBearerToken`) and distributed
   trace headers (`X-Trace-ID`) to forward them in downstream API requests.

```mermaid
flowchart LR
    Client([Incoming HTTP Request]) --> Controller[SprinGo Controller]
    Controller --> Service[Payment Service]
    Service -->|Injects Bean| HTTPClient[Outbound http.Client]
    HTTPClient -->|Propagates JWT & Trace ID| ExternalAPI([Downstream Microservice / Gateway])
```

---

## 2. Configuration Properties

Define client pool properties in `resources/application.yaml`:

```yaml
clients:
  payment-gateway:
    base-url: "https://api.payments.internal.net"
    timeout: 5s
    max-idle-conns: 100
    max-idle-conns-per-host: 20
    idle-conn-timeout: 90s
    tls-handshake-timeout: 5s
```

---

## 3. Registering the HTTP Client Bean

**Suggested File Path**: `internal/infrastructure/config/http_client_config.go`

```go
package config

import (
    "net"
    "net/http"
    "time"

    "github.com/NeftaliAcosta/springo/framework/config"
    "github.com/NeftaliAcosta/springo/framework/ioc"
)

type HttpClientProperties struct {
    BaseUrl             string        `yaml:"base-url"`
    Timeout             time.Duration `yaml:"timeout"`
    MaxIdleConns        int           `yaml:"max-idle-conns"`
    MaxIdleConnsPerHost int           `yaml:"max-idle-conns-per-host"`
    IdleConnTimeout     time.Duration `yaml:"idle-conn-timeout"`
    TLSHandshakeTimeout time.Duration `yaml:"tls-handshake-timeout"`
}

func init() {
    // 1. Bind configuration properties
    props := HttpClientProperties{
        Timeout:             10 * time.Second,
        MaxIdleConns:        100,
        MaxIdleConnsPerHost: 20,
        IdleConnTimeout:     90 * time.Second,
        TLSHandshakeTimeout: 5 * time.Second,
    }
    _ = config.Bind("clients.payment-gateway", &props)

    // 2. Build optimized Transport with connection pooling
    transport := &http.Transport{
        Proxy: http.ProxyFromEnvironment,
        DialContext: (&net.Dialer{
            Timeout:   5 * time.Second,
            KeepAlive: 30 * time.Second,
        }).DialContext,
        ForceAttemptHTTP2:     true,
        MaxIdleConns:          props.MaxIdleConns,
        MaxIdleConnsPerHost:   props.MaxIdleConnsPerHost,
        IdleConnTimeout:       props.IdleConnTimeout,
        TLSHandshakeTimeout:   props.TLSHandshakeTimeout,
        ResponseHeaderTimeout: props.Timeout,
    }

    client := &http.Client{
        Transport: transport,
        Timeout:   props.Timeout,
    }

    // 3. Register client bean in IoC container
    ioc.RegisterBean("paymentHttpClient", client)
}
```

---

## 4. Context & Security Token Propagation

When forwarding calls to downstream microservices, create a custom helper or round-tripper to inject the authenticated
user's JWT token and tracing headers:

**Suggested File Path**: `internal/infrastructure/client/request_helper.go`

```go
package client

import (
    "context"
    "fmt"
    "net/http"

    "github.com/NeftaliAcosta/springo/framework/security"
)

// NewOutboundRequest creates an http.Request with propagated JWT and context cancellation.
func NewOutboundRequest(ctx context.Context, method, url string, body []byte) (*http.Request, error) {
    req, err := http.NewRequestWithContext(ctx, method, url, nil)
    if err != nil {
        return nil, fmt.Errorf("failed to build outbound request: %w", err)
    }

    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("Accept", "application/json")

    // Propagate Bearer JWT token from incoming request context
    if token, ok := security.GetBearerToken(ctx); ok && token != "" {
        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
    }

    // Propagate Trace ID if present in context
    if traceID, ok := ctx.Value("traceId").(string); ok && traceID != "" {
        req.Header.Set("X-Trace-ID", traceID)
    }

    return req, nil
}
```

---

## 5. Consuming the Client in Business Services

Inject the client bean into your service or repository layer:

**Suggested File Path**: `internal/domain/service/payment_service.go`

```go
package service

import (
    "context"
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/NeftaliAcosta/springo/framework/ioc"
    "github.com/your-org/your-app/internal/infrastructure/client"
)

type PaymentGatewayClient struct {
    httpClient *http.Client
    baseURL    string
}

func init() {
    ioc.RegisterBean("paymentGatewayClient", &PaymentGatewayClient{
        baseURL: "https://api.payments.internal.net",
    })
}

// PostConstruct resolves dependencies from the IoC container.
func (s *PaymentGatewayClient) PostConstruct() {
    if c, ok := ioc.Get[*http.Client]("paymentHttpClient"); ok {
        s.httpClient = c
    }
}

type PaymentResponse struct {
    TransactionID string `json:"transactionId"`
    Status        string `json:"status"`
}

func (s *PaymentGatewayClient) ProcessPayment(ctx context.Context, orderID string, amount float64) (*PaymentResponse, error) {
    url := fmt.Sprintf("%s/v1/charges", s.baseURL)

    req, err := client.NewOutboundRequest(ctx, http.MethodPost, url, nil)
    if err != nil {
        return nil, err
    }

    resp, err := s.httpClient.Do(req)
    if err != nil {
        return nil, fmt.Errorf("outbound payment call failed: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
        return nil, fmt.Errorf("payment gateway returned status %d", resp.StatusCode)
    }

    var result PaymentResponse
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("failed to decode gateway response: %w", err)
    }

    return &result, nil
}
```
