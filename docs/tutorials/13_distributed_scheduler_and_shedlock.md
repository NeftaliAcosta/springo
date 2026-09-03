# ⏰ Step-by-Step Guide: Distributed Scheduling & ShedLock

This tutorial explains how to run recurring background cron tasks with distributed locking across multi-replica
deployments in SprinGo.

---

## 1. Overview

SprinGo provides a cluster-aware background task scheduler:
- **Cron Expressions**: Standard 5-field/6-field cron schedules or interval durations (e.g. `@every 5m`).
- **ShedLock Distributed Locking**: Uses the database to guarantee that exactly one pod executes the task at any time.
- **Automatic Lock Expiration**: Protects against node crashes by enforcing maximum lock durations.
- **Graceful Shutdown Integration**: Waits for executing cron jobs to finish cleanly during process termination.

---

## 2. Registering Scheduled Tasks

**Suggested File Path**: `internal/infrastructure/input/scheduler/report_jobs.go`
```go
package scheduler

import (
    "context"
    "log/slog"
    "time"

    "github.com/NeftaliAcosta/springo/framework/scheduler"
)

func init() {
    // Schedule task every night at 2:00 AM UTC with ShedLock
    scheduler.Schedule("generate-daily-reports", "0 2 * * *", generateDailyReports,
        scheduler.WithLockAtMostFor(10*time.Minute),
        scheduler.WithLockAtLeastFor(30*time.Second),
    )

    // Schedule task every 10 minutes
    scheduler.Every("sync-exchange-rates", 10*time.Minute, syncExchangeRates,
        scheduler.WithLockAtMostFor(2*time.Minute),
    )
}

func generateDailyReports(ctx context.Context) error {
    slog.Info("Running daily report generation job...")
    // Business logic here
    return nil
}

func syncExchangeRates(ctx context.Context) error {
    slog.Info("Syncing latest exchange rates from external API...")
    return nil
}
```

---

## 3. Configuration

**Suggested File Path**: `resources/application.yaml`
```yaml
spring:
  scheduler:
    enabled: true
    pool-size: 5
    lock-table-name: springo_shedlock
```
