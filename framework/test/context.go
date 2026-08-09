package test

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework"
	"github.com/NeftaliAcosta/springo/framework/event"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
	"github.com/NeftaliAcosta/springo/framework/web"
	"os"
	"testing"

	"gorm.io/gorm"
)

// SprinGoTestContext represents the isolated testing environment
type SprinGoTestContext struct {
	App      *framework.Application
	Client   *TestClient
	tx       *gorm.DB // The active transaction for auto-rollback
	teardown func()
}

// NewSprinGoTestContext bootstraps the framework in test mode.
// It forces the "test" profile and initiates a global database transaction.
func NewSprinGoTestContext(t *testing.T, opts ...framework.Options) *SprinGoTestContext {
	// Force test profile so it loads application-test.yaml if exists, or defaults
	_ = os.Setenv("SPRINGO_PROFILES_ACTIVE", "test")

	// 1. Bootstrap the core application
	app := framework.Bootstrap(opts...)

	// 2. Database Isolation (Auto-Rollback mechanism)
	var tx *gorm.DB
	if app.DB != nil {
		tx = app.DB.Begin()
		if tx.Error != nil {
			t.Fatalf("Failed to start test transaction: %v", tx.Error)
		}
		// Replace the global DB with the transaction so all queries happen inside it
		ioc.GetContainer().SetDB(tx)
	}

	// 3. Create Fluent Client
	client := NewTestClient(t, app.Router)

	return &SprinGoTestContext{
		App:    app,
		Client: client,
		tx:     tx,
		teardown: func() {
			// Rollback to keep database clean
			if tx != nil {
				tx.Rollback()
			}
			// Reset container state for the next test
			ioc.GetContainer().Clear()
			// Clean up event worker pool
			event.StopWorkerPool()
			// Clean up scheduler background jobs
			scheduler.StopBackgroundJobs()
			// Clean up telemetry exporter
			web.CloseTelemetry()
		},
	}
}

// TearDown should be called using defer immediately after NewSprinGoTestContext
func (ctx *SprinGoTestContext) TearDown() {
	if ctx.teardown != nil {
		ctx.teardown()
	}
}

// ReplaceBean is a shortcut for mocking beans in the IoC container during testing
func (ctx *SprinGoTestContext) ReplaceBean(name string, mock interface{}) {
	ioc.GetContainer().ReplaceBean(name, mock)
}

// GetContext returns a standard context for passing into service methods
func (ctx *SprinGoTestContext) GetContext() context.Context {
	return context.Background()
}
