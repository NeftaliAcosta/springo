package framework

import (
	"context"
	"errors"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/database"
	"github.com/NeftaliAcosta/springo/framework/event"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/lifecycle"
	"github.com/NeftaliAcosta/springo/framework/logging"
	"github.com/NeftaliAcosta/springo/framework/scheduler"
	"github.com/NeftaliAcosta/springo/framework/web"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	httpSwagger "github.com/swaggo/http-swagger"
	"gorm.io/gorm"
)

// Options defines framework-level configurations
type Options struct {
	Middlewares   []func(http.Handler) http.Handler
	DebugLogs     bool
	DisableBanner bool
	ConfigDir     string
}

// Application represents the SprinGo application context
type Application struct {
	DB           *gorm.DB
	Loader       *config.ConfigLoader
	Router       chi.Router
	Debug        bool
	StartTime    time.Time
	mu           sync.RWMutex
	httpServer   *http.Server
	listener     net.Listener
	shutdownOnce sync.Once
	shutdownErr  error
	readyChan    chan struct{}
}

// Ready returns a channel that is closed when the HTTP server is running and listening.
func (a *Application) Ready() <-chan struct{} {
	return a.readyChan
}

// BootstrapE initializes the core components of the framework and returns an error on failure.
func BootstrapE(opts ...Options) (*Application, error) {
	startTime := time.Now()
	web.SetStartTime(startTime)

	var cleanups []func()
	var success bool
	defer func() {
		if !success {
			executeCleanups(cleanups)
		}
	}()

	// Parse options
	opt := parseOptions(opts)

	// 1. Show Identity Banner
	showBanner(opt.DisableBanner)

	// 2. Load Configuration Engine
	loader := config.NewConfigLoader().WithBaseDir(opt.ConfigDir)
	if err := loader.Load(); err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}

	// 3. Automated Properties Binding
	if err := config.InitializeProperties(loader); err != nil {
		return nil, fmt.Errorf("failed to initialize properties: %w", err)
	}

	// 4. Evaluate Conditional Registrations (@ConditionalOnProperty / When)
	config.ExecuteConditionals(loader)

	// 5. Initialize Structured Logging
	logging.Initialize(config.Get[logging.LoggingProperties]())

	// 6. Initialize i18n MessageSource
	initI18n(opt.ConfigDir)

	// 7. Initialize Health Actuator (Opt-in setup)
	web.InitializeHealthIndicators()

	// 8. Initialize Telemetry Exporter (W3C / Zipkin)
	initTelemetry(&cleanups)

	// 9. Automated Database Connection & Migrations
	dbConn, err := initDatabase(opt.DebugLogs, opt.ConfigDir, &cleanups)
	if err != nil {
		return nil, err
	}

	// 10. Connect Additional DataSources
	if err := initAdditionalDataSources(opt.DebugLogs, &cleanups); err != nil {
		return nil, err
	}

	// 11. Initialize IoC Container (for Services/Repositories)
	ioc.GetContainer().InitializeAllBeans(dbConn)

	// 12. Execute fail-fast application initializers
	cleanups = append(cleanups, func() {
		_ = lifecycle.RunShutdown(context.Background())
	})
	if err := lifecycle.RunInitializers(context.Background()); err != nil {
		return nil, fmt.Errorf("application initialization failed: %w", err)
	}

	// 13. Run Critical Startup Jobs
	if err := scheduler.RunStartupTasksE(); err != nil {
		return nil, fmt.Errorf("failed to run startup tasks: %w", err)
	}

	// 14. Create Router with Standard + Custom Middlewares & Infrastructure Routes
	r := setupRouter(opt.Middlewares)

	success = true
	return &Application{
		DB:        dbConn,
		Loader:    loader,
		Router:    r,
		Debug:     opt.DebugLogs,
		StartTime: startTime,
		readyChan: make(chan struct{}),
	}, nil
}

func parseOptions(opts []Options) Options {
	opt := Options{ConfigDir: "resources"}
	if len(opts) > 0 {
		opt = opts[0]
		if opt.ConfigDir == "" {
			opt.ConfigDir = "resources"
		}
	}
	return opt
}

func showBanner(disableBanner bool) {
	if disableBanner {
		return
	}
	loggingProps := config.Get[logging.LoggingProperties]()
	if loggingProps != nil && !loggingProps.IsBannerEnabled() {
		return
	}
	if os.Getenv("SPRINGO_BANNER_MODE") != "off" && os.Getenv("SPRINGO_PROFILES_ACTIVE") != "test" {
		web.ShowBanner()
	}
}

func executeCleanups(cleanups []func()) {
	for i := len(cleanups) - 1; i >= 0; i-- {
		cleanups[i]()
	}
}

func setupRouter(middlewares []func(http.Handler) http.Handler) chi.Router {
	r := web.CreateDefaultRouter(func(r chi.Router) {
		for _, m := range middlewares {
			r.Use(m)
		}
	})

	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("SprinGo - Engine active. Check /swagger/index.html"))
	})
	r.Get("/swagger/*", httpSwagger.WrapHandler)

	return r
}

func initI18n(configDir string) {
	i18nProps := config.Get[web.I18nProperties]()
	defaultLocale := "en"
	path := configDir
	if i18nProps != nil {
		if i18nProps.DefaultLocale != "" {
			defaultLocale = i18nProps.DefaultLocale
		}
		if i18nProps.Path != "" {
			path = i18nProps.Path
		}
	}
	messageSource := web.NewMessageSource(defaultLocale)
	if err := messageSource.LoadTranslations(path); err != nil {
		slog.Warn("Failed to load translations",
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.String("path", path),
			slog.Any("error", err),
		)
	}
	ioc.GetContainer().RegisterBean("messageSource", messageSource)
}

func initTelemetry(cleanups *[]func()) {
	telemetryProps := config.Get[web.TelemetryProperties]()
	if telemetryProps != nil && telemetryProps.Enabled {
		web.InitializeTelemetry(telemetryProps)
		*cleanups = append(*cleanups, func() {
			web.CloseTelemetry()
		})
	}
}

func initDatabase(debug bool, configDir string, cleanups *[]func()) (*gorm.DB, error) {
	dbProps := config.Get[database.DataSourceProperties]()
	if dbProps == nil || dbProps.Driver == "" {
		return nil, nil
	}

	dbConn, err := database.Connect(dbProps)
	if err != nil {
		return nil, fmt.Errorf("primary database connection failed (%s): %w", dbProps.Driver, err)
	}
	*cleanups = append(*cleanups, func() {
		if sqlDB, err := dbConn.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	// Set custom migration table if defined
	database.SetMigrationTableName(dbProps.MigrationTable)

	if dbProps.AutoMigrate {
		migrator := database.NewMigrationManager(dbConn, debug)
		if err := migrator.Migrate(); err != nil {
			return nil, fmt.Errorf("failed to run migrations on primary DB: %w", err)
		}
	}

	// Auto-load schema.sql and data.sql in dev/test/default profiles
	profile := os.Getenv("SPRINGO_PROFILES_ACTIVE")
	if profile == "dev" || profile == "test" || profile == "" {
		if err := database.ExecuteSQLFile(dbConn, filepath.Join(configDir, "schema.sql")); err != nil {
			return nil, fmt.Errorf("failed to run schema.sql on primary DB: %w", err)
		}
		if err := database.ExecuteSQLFile(dbConn, filepath.Join(configDir, "data.sql")); err != nil {
			return nil, fmt.Errorf("failed to run data.sql on primary DB: %w", err)
		}
	}

	initEventBus(dbConn, cleanups)
	initScheduler(dbConn)

	return dbConn, nil
}

func initEventBus(dbConn *gorm.DB, cleanups *[]func()) {
	eventProps := config.Get[event.EventProperties]()
	if eventProps != nil && eventProps.Enabled {
		if eventProps.DLQ.Enabled {
			if err := dbConn.AutoMigrate(&event.FailedEventEntity{}); err != nil {
				slog.Warn("Failed to run DLQ migrations",
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
					slog.Any("error", err),
				)
			} else {
				slog.Info("DLQ Infrastructure ready (springo_failed_events)",
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
				)
			}
		}
		if eventProps.Outbox.Enabled {
			if err := dbConn.AutoMigrate(&event.OutboxEventEntity{}); err != nil {
				slog.Warn("Failed to run Outbox migrations",
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
					slog.Any("error", err),
				)
			} else {
				slog.Info("Outbox Infrastructure ready (springo_outbox)",
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
				)
			}
		}
		event.PrintEventMap()
		event.StartWorkerPool(eventProps)
		*cleanups = append(*cleanups, func() {
			event.StopWorkerPool()
		})
	}
}

func initScheduler(dbConn *gorm.DB) {
	schedulerProps := config.Get[scheduler.SchedulerProperties]()
	if schedulerProps != nil && schedulerProps.Enabled {
		hasLock := false
		for _, job := range schedulerProps.Jobs {
			if job.Lock.Enabled {
				hasLock = true
				break
			}
		}
		if hasLock {
			if err := dbConn.AutoMigrate(&scheduler.ShedLockEntity{}); err != nil {
				slog.Warn("Failed to run ShedLock migrations",
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
					slog.Any("error", err),
				)
			} else {
				slog.Info("ShedLock Infrastructure ready (springo_shedlock)",
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
				)
			}
		}
	}
}

func initAdditionalDataSources(debug bool, cleanups *[]func()) error {
	additionalProps := config.Get[database.AdditionalDataSources]()
	if additionalProps == nil {
		return nil
	}

	for name, props := range *additionalProps {
		if err := initSingleDataSource(name, props, debug, cleanups); err != nil {
			return err
		}
	}
	return nil
}

func initSingleDataSource(name string, props database.DataSourceProperties, debug bool, cleanups *[]func()) error {
	conn, err := database.Connect(&props)
	if err != nil {
		return fmt.Errorf("additional database connection failed (%s: %s): %w", name, props.Driver, err)
	}
	ioc.GetContainer().RegisterBean(name, conn)
	slog.Info("Connected to additional datasource",
		slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
		slog.String("name", name),
	)

	*cleanups = append(*cleanups, func() {
		if sqlDB, err := conn.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	if props.AutoMigrate {
		migrator := database.NewMigrationManager(conn, debug)
		if err := migrator.Migrate(); err != nil {
			return fmt.Errorf("failed to run migrations on %s: %w", name, err)
		}
	}
	return nil
}

// Bootstrap initializes the core components of the framework
func Bootstrap(opts ...Options) *Application {
	app, err := BootstrapE(opts...)
	if err != nil {
		slog.Error("Bootstrap failed",
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.Any("error", err),
		)
		os.Exit(1)
	}
	return app
}

// Start launches the web server by delegating to Run
func (a *Application) Start() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := a.runAndShutdown(ctx); err != nil {
		slog.Error("Server startup failed",
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.Any("error", err),
		)
		os.Exit(1)
	}
}

func (a *Application) runAndShutdown(ctx context.Context) error {
	runErr := a.Run(ctx)
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	shutdownErr := a.Shutdown(shutdownCtx)
	return errors.Join(runErr, shutdownErr)
}

// Run executes command line runners, background tasks, and starts serving on dynamic port securely
func (a *Application) Run(ctx context.Context) error {
	scheduler.StartBackgroundJobs()

	// Execute CommandLineRunner beans post-startup
	a.RunCommandLineRunners(os.Args[1:])

	type ServerProps struct {
		Port int `yaml:"port"`
	}
	props := &ServerProps{}
	_ = a.Loader.BindPrefix("server", props)

	port := props.Port
	if port == 0 && os.Getenv("SPRINGO_PROFILES_ACTIVE") != "test" {
		port = 8080
	}

	server, ln, err := web.BuildServer(port, a.Router)
	if err != nil {
		return err
	}

	// If port 0 was requested (dynamically allocated), update the server address with the real port
	if tcpAddr, ok := ln.Addr().(*net.TCPAddr); ok && props.Port == 0 {
		server.Addr = fmt.Sprintf(":%d", tcpAddr.Port)
	}

	a.mu.Lock()
	a.httpServer = server
	a.listener = ln
	a.mu.Unlock()

	if err := lifecycle.RunReady(ctx); err != nil {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = a.Shutdown(shutdownCtx)
		return fmt.Errorf("application ready hook failed: %w", err)
	}
	close(a.readyChan) // Signal server and ready hooks completed successfully.

	// Monitor context cancellation
	stopCtxChan := make(chan struct{})
	defer close(stopCtxChan)

	go func() {
		select {
		case <-ctx.Done():
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = a.Shutdown(shutdownCtx)
		case <-stopCtxChan:
			// Run returned normally, exit monitoring
		}
	}()

	slog.Info("SprinGo Server running",
		slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
		slog.String("url", "http://localhost"+server.Addr),
	)

	if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// GetHttpServer retrieves the running HTTP server thread-safely
func (a *Application) GetHttpServer() *http.Server {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.httpServer
}

// GetListener retrieves the running TCP listener thread-safely
func (a *Application) GetListener() net.Listener {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.listener
}

// Shutdown coordinates a clean and graceful shutdown of all subsystems (idempotent)
func (a *Application) Shutdown(ctx context.Context) error {
	a.shutdownOnce.Do(func() {
		a.shutdownErr = a.doShutdown(ctx)
	})
	return a.shutdownErr
}

func (a *Application) doShutdown(ctx context.Context) error {
	slog.Info("Shutting down SprinGo application context", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	var errs []error

	a.mu.Lock()
	srv := a.httpServer
	a.mu.Unlock()

	// 1. Stop accepting new HTTP requests and shutdown HTTP server gracefully
	a.shutdownHTTPServer(ctx, srv)

	// 2. Stop scheduler background tasks
	slog.Info("Stopping background scheduler", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	scheduler.StopBackgroundJobs()

	// 3. Stop event bus worker pool
	slog.Info("Stopping event bus worker pool", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	event.StopWorkerPool()

	// 4. Execute application shutdown hooks in reverse order
	if err := lifecycle.RunShutdown(ctx); err != nil {
		errs = append(errs, err)
	}

	// 5. Close telemetry
	slog.Info("Closing telemetry", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	web.CloseTelemetry()

	// 6. Close additional datasources and io.Closer beans
	errs = append(errs, a.closeAdditionalBeans()...)

	// 7. Close primary database connection
	if err := a.closePrimaryDB(); err != nil {
		errs = append(errs, err)
	}

	// 8. Clear IoC Container global instances
	slog.Info("Clearing IoC container", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	ioc.GetContainer().Clear()

	if len(errs) > 0 {
		return fmt.Errorf("graceful shutdown completed with errors: %v", errs)
	}

	slog.Info("SprinGo application context shutdown complete", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	return nil
}

func (a *Application) shutdownHTTPServer(ctx context.Context, srv *http.Server) {
	if srv != nil {
		slog.Info("Stopping HTTP server", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
		if err := srv.Shutdown(ctx); err != nil {
			slog.Warn("HTTP server graceful shutdown timed out, forcing close",
				slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
				slog.Any("error", err),
			)
			_ = srv.Close()
		}
	}
}

func (a *Application) closeAdditionalBeans() []error {
	var errs []error
	slog.Info("Closing additional beans and resources", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	beans := ioc.GetContainer().GetAllBeans()
	for name, bean := range beans {
		if err := a.closeBean(name, bean); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

func (a *Application) closeBean(name string, bean any) error {
	if db, ok := bean.(*gorm.DB); ok {
		if db != a.DB {
			if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
				slog.Info("Closing datasource bean",
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
					slog.String("name", name),
				)
				if err := sqlDB.Close(); err != nil {
					return fmt.Errorf("error closing additional database %s: %w", name, err)
				}
			}
		}
	} else if closer, ok := bean.(io.Closer); ok {
		slog.Info("Closing bean",
			slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
			slog.String("name", name),
		)
		if err := closer.Close(); err != nil {
			return fmt.Errorf("error closing bean %s: %w", name, err)
		}
	}
	return nil
}

func (a *Application) closePrimaryDB() error {
	slog.Info("Closing database connections", slog.String(logging.SubsystemKey, logging.FrameworkSubsystem))
	if a.DB != nil {
		if sqlDB, err := a.DB.DB(); err == nil {
			if err := sqlDB.Close(); err != nil {
				return fmt.Errorf("DB close error: %w", err)
			}
		}
	}
	return nil
}

