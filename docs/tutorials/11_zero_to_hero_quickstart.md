# 🚀 Zero-to-Production: Complete Beginner Guide for SprinGo

This tutorial guides you from installing the tools to deploying your first enterprise microservice in production.

---

## 1. Prerequisites

1. **Install Go** (version 1.22 or higher):
   ```bash
   # Verify Go version
   go version
   ```

2. **Install SprinGo CLI**:
   ```bash
   go install github.com/NeftaliAcosta/springo/cmd/springo@v1.0.0-rc18
   ```

---

## 2. Create Your First Microservice

```bash
# Scaffold the project
springo new store-api

# Enter the project directory
cd store-api
```

---

## 3. Generate Domain Components

Use the `springo make` commands to create domain components in Hexagonal Architecture:

```bash
# 1. Domain Model
springo make model Product

# 2. Request & Response DTOs
springo make dto Product

# 3. Repository Port & Adapter
springo make repository Product

# 4. Use Case & Application Service
springo make service Product

# 5. REST Controller
springo make controller Product

# 6. Database Migration
springo make migration CreateProductsTable
```

---

## 4. Define Database Schema Migration

Edit `resources/db/migration/V1.0__create_products_table.sql`:

```sql
CREATE TABLE IF NOT EXISTS products (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    name VARCHAR(200) NOT NULL,
    price DECIMAL(10, 2) NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

---

## 5. Run the Application Locally

```bash
# Run database migrations
springo migrate

# Launch local server
go run cmd/app/main.go
```

Your API is now running at `http://localhost:8080`!
- **Health Check**: `http://localhost:8080/actuator/health`
- **Admin Dashboard**: `http://localhost:8080/actuator/dashboard`
- **Swagger Docs**: `http://localhost:8080/swagger/index.html`

---

## 6. Docker & Production Deployment

### 6.1 Multi-Stage Dockerfile
**Suggested File Path**: `Dockerfile`
```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main ./cmd/app/main.go

FROM alpine:3.21
WORKDIR /app
COPY --from=builder /app/main .
COPY --from=builder /app/resources ./resources
EXPOSE 8080
ENV SPRINGO_PROFILES_ACTIVE=prod
ENTRYPOINT ["./main"]
```

### 6.2 Build and Run Container
```bash
# Build Docker image
docker build -t store-api:latest .

# Run with PostgreSQL datasource
docker run -p 8080:8080 \
  -e SPRINGO_PROFILES_ACTIVE=prod \
  -e DATABASE_URL="postgres://user:pass@host:5432/storedb" \
  store-api:latest
```
