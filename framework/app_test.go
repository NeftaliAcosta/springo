package framework

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/lifecycle"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestApplication_Lifecycle(t *testing.T) {
	// Restore registrations and configs to prevent test contamination
	restore := scheduler.BackupRegistrations()
	t.Cleanup(restore)
	restoreLifecycle := lifecycle.BackupRegistrations()
	t.Cleanup(restoreLifecycle)
	t.Cleanup(func() {
		ioc.GetContainer().Clear()
		config.ResetProperties()
	})

	// Set test profile so it doesn't load physical schema/data files
	os.Setenv("SPRINGO_PROFILES_ACTIVE", "test")
	defer os.Unsetenv("SPRINGO_PROFILES_ACTIVE")

	var lifecycleCalls []string
	lifecycle.RegisterInitializer("test-initializer", 0, func(context.Context) error {
		lifecycleCalls = append(lifecycleCalls, "initializer")
		return nil
	})
	lifecycle.RegisterReady("test-ready", 0, func(context.Context) error {
		lifecycleCalls = append(lifecycleCalls, "ready")
		return nil
	})
	lifecycle.RegisterShutdown("test-shutdown", 0, func(context.Context) error {
		lifecycleCalls = append(lifecycleCalls, "shutdown")
		return nil
	})

	// Bootstrap application using BootstrapE instead of Bootstrap to avoid os.Exit on error
	app, err := BootstrapE(Options{
		DisableBanner: true,
	})
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.NotNil(t, app.Router)

	// Run application in background
	errChan := make(chan error, 1)
	ctx, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()

	go func() {
		errChan <- app.Run(ctx)
	}()

	// Wait deterministically for server to be ready
	select {
	case <-app.Ready():
		// Server started and listener is active
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for application to become ready")
	}

	// Verify server started and listener is active
	assert.NotNil(t, app.GetHttpServer())
	assert.NotNil(t, app.GetListener())

	// Trigger clean shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = app.Shutdown(shutdownCtx)
	assert.NoError(t, err)
	assert.Equal(t, []string{"initializer", "ready", "shutdown"}, lifecycleCalls)

	// Expect Run() to have returned http.ErrServerClosed or nil
	select {
	case runErr := <-errChan:
		assert.True(t, runErr == nil || runErr == http.ErrServerClosed)
	case <-time.After(2 * time.Second):
		t.Fatal("Timeout waiting for application to exit Run loop")
	}
}

func TestApplication_BootstrapE_Success(t *testing.T) {
	restore := scheduler.BackupRegistrations()
	t.Cleanup(restore)
	restoreLifecycle := lifecycle.BackupRegistrations()
	t.Cleanup(restoreLifecycle)
	t.Cleanup(func() {
		ioc.GetContainer().Clear()
		config.ResetProperties()
	})

	os.Setenv("SPRINGO_PROFILES_ACTIVE", "test")
	defer os.Unsetenv("SPRINGO_PROFILES_ACTIVE")

	app, err := BootstrapE(Options{
		DisableBanner: true,
	})
	assert.NoError(t, err)
	assert.NotNil(t, app)
	assert.NotNil(t, app.Router)
}

func TestApplication_BootstrapE_Failure(t *testing.T) {
	restore := scheduler.BackupRegistrations()
	t.Cleanup(restore)
	restoreLifecycle := lifecycle.BackupRegistrations()
	t.Cleanup(restoreLifecycle)
	t.Cleanup(func() {
		ioc.GetContainer().Clear()
		config.ResetProperties()
	})

	os.Setenv("SPRINGO_PROFILES_ACTIVE", "test")
	defer os.Unsetenv("SPRINGO_PROFILES_ACTIVE")

	scheduler.Register("AuthTokenJob", func(ctx context.Context) error {
		return fmt.Errorf("mocked critical auth token failure")
	})

	app, err := BootstrapE(Options{
		DisableBanner: true,
	})
	assert.Error(t, err)
	assert.Nil(t, app)
	assert.Contains(t, err.Error(), "mocked critical auth token failure")
}

func TestApplication_BootstrapE_InitializerFailure(t *testing.T) {
	restore := scheduler.BackupRegistrations()
	t.Cleanup(restore)
	restoreLifecycle := lifecycle.BackupRegistrations()
	t.Cleanup(restoreLifecycle)
	t.Cleanup(func() {
		ioc.GetContainer().Clear()
		config.ResetProperties()
	})

	os.Setenv("SPRINGO_PROFILES_ACTIVE", "test")
	defer os.Unsetenv("SPRINGO_PROFILES_ACTIVE")

	lifecycle.RegisterInitializer("failing-initializer", 0, func(context.Context) error {
		return fmt.Errorf("mocked initializer failure")
	})

	app, err := BootstrapE(Options{DisableBanner: true})
	assert.Error(t, err)
	assert.Nil(t, app)
	assert.Contains(t, err.Error(), "mocked initializer failure")
}

func TestApplication_Run_ReadyFailureShutsDown(t *testing.T) {
	restore := scheduler.BackupRegistrations()
	t.Cleanup(restore)
	restoreLifecycle := lifecycle.BackupRegistrations()
	t.Cleanup(restoreLifecycle)
	t.Cleanup(func() {
		ioc.GetContainer().Clear()
		config.ResetProperties()
	})

	os.Setenv("SPRINGO_PROFILES_ACTIVE", "test")
	defer os.Unsetenv("SPRINGO_PROFILES_ACTIVE")

	shutdownCalled := false
	lifecycle.RegisterReady("failing-ready", 0, func(context.Context) error {
		return fmt.Errorf("mocked ready failure")
	})
	lifecycle.RegisterShutdown("test-shutdown", 0, func(context.Context) error {
		shutdownCalled = true
		return nil
	})

	app, err := BootstrapE(Options{DisableBanner: true})
	assert.NoError(t, err)

	err = app.Run(context.Background())
	assert.ErrorContains(t, err, "mocked ready failure")
	assert.True(t, shutdownCalled)
	select {
	case <-app.Ready():
		t.Fatal("Ready channel must remain open when a ready hook fails")
	default:
	}
}
