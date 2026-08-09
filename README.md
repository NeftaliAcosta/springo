# SprinGo Framework 🚀

<p align="center">
  <b>High-performance, opinionated Go framework for Spring Boot developers.</b>
</p>

<p align="center">
  <a href="https://github.com/NeftaliAcosta/springo/actions/workflows/ci.yml"><img src="https://github.com/NeftaliAcosta/springo/actions/workflows/ci.yml/badge.svg" alt="CI Status"></a>
  <a href="https://pkg.go.dev/github.com/NeftaliAcosta/springo"><img src="https://pkg.go.dev/badge/github.com/NeftaliAcosta/springo.svg" alt="Go Reference"></a>
  <a href="https://github.com/NeftaliAcosta/springo/releases"><img src="https://img.shields.io/github/v/release/NeftaliAcosta/springo?include_prereleases&color=blue" alt="Release"></a>
  <a href="https://goreportcard.com/report/github.com/NeftaliAcosta/springo"><img src="https://goreportcard.com/badge/github.com/NeftaliAcosta/springo" alt="Go Report Card"></a>
  <a href="https://opensource.org/licenses/MIT"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
</p>

<p align="center">
  <img src="https://img.shields.io/github/stars/NeftaliAcosta/springo?style=social" alt="Stars">
  <img src="https://img.shields.io/github/forks/NeftaliAcosta/springo?style=social" alt="Forks">
  <img src="https://img.shields.io/github/issues/NeftaliAcosta/springo" alt="Issues">
  <img src="https://img.shields.io/github/contributors/NeftaliAcosta/springo" alt="Contributors">
</p>

---

> ⚠️ **Release status:** `v1.0.0-rc1` is a Release Candidate. Validate it in a staging environment before adopting it for production workloads; public APIs may still receive release-blocking corrections before `v1.0.0`.

---

## ⚡ Quick Start

### 1. Install SprinGo CLI

```bash
go install github.com/NeftaliAcosta/springo/cmd/cli@v1.0.0-rc1
```

### 2. Scaffold a New Enterprise Service

```bash
springo new my-service
cd my-service
go run cmd/app/main.go
```

Your API is now live at `http://localhost:8080` with Actuator Dashboard at `http://localhost:8080/actuator/dashboard`! 🎉

---

## 🚀 Key Features

| Category | Feature | Description |
| :--- | :--- | :--- |
| 🛠️ **CLI Tooling** | **Code Generators & Scaffolding** | `springo new` and `springo make` for instant Hexagonal Architecture components. |
| 🎛️ **Management** | **Spring-style Actuator** | Embedded Glassmorphic UI Dashboard for Health, Goroutine Dumps, Beans & DLQ. |
| 🧩 **Core Engine** | **IoC & Auto-Wiring** | Dependency injection container with reflection & tag-based field autowiring (`spring:"beanName"`). |
| 🔒 **Security** | **Enterprise JWT & CSRF** | Support for HS256, RS256 (Keycloak/Auth0 JWKS), OWASP Security Headers & CSRF. |
| ⚡ **Database** | **GORM & ShedLock** | Declarative transactions with `REQUIRED` propagation and cluster-wide cron locking. |
| 📡 **Messaging** | **Event Bus & Outbox/DLQ** | Domain Pub/Sub with Outbox buffer, automatic retries, and Dead Letter Queue management. |
| ⚙️ **Config** | **Profiles & Validation** | Fail-fast property validation with `application-{profile}.yaml` environments. |

---

## 🏗️ Architecture

SprinGo enforces a **Flattened Professional Hexagonal Architecture** with a dedicated **Kernel**:

```text
springo/
├── framework/                 # 🛠️ THE KERNEL (SprinGo Engine Library)
│   ├── app.go
│   ├── runner.go
│   ├── cache/                 # Multi-provider cache abstraction
│   ├── config/                # YAML Profile loader & validation
│   ├── database/              # GORM datasource & ShedLock migrator
│   ├── event/                 # Pub/Sub EventBus, Outbox & DLQ
│   ├── ioc/                   # Dependency Injection Container
│   ├── scheduler/             # Cron manager with ShedLock
│   ├── security/              # JWT & LDAP providers
│   └── web/                   # Chi router, Actuator & Validation
├── cmd/
│   └── cli/                   # 🛠️ SprinGo CLI (v1.0.0-rc1)
├── demo-api/                  # 🚀 Reference Application
└── README.md
```

---

## 🛠️ SprinGo CLI (`springo`)

The `springo` CLI automates daily development workflows:

### Scaffolding & Code Generation

```bash
# 1. Create a new microservice
springo new billing-service

# 2. Generate Hexagonal domain components
springo make model Invoice
springo make dto Invoice
springo make repository Invoice
springo make service Invoice
springo make controller Invoice
springo make migration CreateInvoicesTable
```

### Database Migrations & Route Discovery

```bash
# Migration controls
springo migrate
springo migrate status
springo migrate rollback --steps=1

# Terminal route discovery
springo routes
```

---

## 📊 Actuator Dashboard

SprinGo includes an embedded, zero-dependency Web Console inspired by **Spring Boot Admin**:

- **Health Engine**: Automatic discovery and status checks for all GORM datasources & Redis connections.
- **Goroutine Dump**: Real-time stack trace inspection for concurrency debugging.
- **Bean Directory**: Full visibility into active IoC container definitions.
- **Dead Letter Queue**: Web interface to inspect, retry (re-dispatch), or purge failed domain events.

Access it locally at: `http://localhost:8080/actuator/dashboard`

---

## ⚙️ Configuration & Profiles

Configuration is **100% optional**. If omitted, SprinGo falls back to sensible enterprise defaults. Load specific profile files via `SPRINGO_PROFILES_ACTIVE`:

```bash
# Development profile (loads resources/application-dev.yaml)
SPRINGO_PROFILES_ACTIVE=dev go run cmd/app/main.go

# Production profile (loads resources/application-prod.yaml)
SPRINGO_PROFILES_ACTIVE=prod ./main
```

---

## 🔐 Security & Safety

- **Production Hardening**: Production profile automatically enforces 256-bit JWT secret entropy.
- **Demo Safety**: `demo-api/` is a separate test module. Its development JWT secrets are rejected automatically in `prod` profiles.

---

## 🤝 Contributors

Thank you to all the people who contribute to **SprinGo Framework**!

<a href="https://github.com/NeftaliAcosta/springo/graphs/contributors">
  <img src="https://contrib.rocks/image?repo=NeftaliAcosta/springo" alt="Contributors" />
</a>

---

## 📄 License

SprinGo Framework is open-source software licensed under the **[MIT License](LICENSE)**.
