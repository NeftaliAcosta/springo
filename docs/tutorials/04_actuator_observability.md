# 📊 Step-by-Step Guide: Actuator & Observability in SprinGo

This tutorial explains how to monitor, diagnose, and inspect SprinGo microservices using Actuator endpoints.

---

## 1. Overview

SprinGo Actuator provides production-ready monitoring tools:
- **Health Probes**: Unauthenticated `/actuator/health` probe for Kubernetes liveness/readiness.
- **Embedded Web Console**: Glassmorphic dashboard UI at `/actuator/dashboard`.
- **Runtime Diagnostics**: Endpoints for Goroutine dumps, IoC Beans, Environment properties, and Metrics.
- **Dead Letter Queue (DLQ)**: Web console to inspect, retry, or purge failed asynchronous events.

---

## 2. Configuration & Security

**Suggested File Path**: `resources/application.yaml`
```yaml
management:
  endpoints:
    web:
      exposure:
        include: "health,info,beans,goroutines,metrics,dlq"
      base-path: /actuator
  security:
    enabled: true
    username: admin
    password: ${ACTUATOR_PASSWORD:secret123}
```

---

## 3. Available Endpoints

| Endpoint | Method | Security | Description |
| :--- | :--- | :--- | :--- |
| `/actuator/health` | `GET` | Public / Basic Auth | Minimal `{"status": "UP"}` or detailed subsystem checks. |
| `/actuator/dashboard` | `GET` | Basic Auth | Embedded Glassmorphic Admin UI console. |
| `/actuator/beans` | `GET` | Basic Auth | Directory of all registered IoC beans and singletons. |
| `/actuator/goroutines` | `GET` | Basic Auth | Live stack trace dump for concurrency debugging. |
| `/actuator/metrics` | `GET` | Basic Auth | System, memory, and GC telemetry metrics. |
| `/actuator/dlq` | `GET, POST` | Basic Auth | Inspect and re-dispatch failed domain events. |

---

## 4. Kubernetes Probe Configuration

**Suggested File Path**: `k8s/deployment.yaml`
```yaml
livenessProbe:
  httpGet:
    path: /actuator/health
    port: 8080
  initialDelaySeconds: 10
  periodSeconds: 10

readinessProbe:
  httpGet:
    path: /actuator/health
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```
