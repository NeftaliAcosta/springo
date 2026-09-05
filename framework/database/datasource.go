package database

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/NeftaliAcosta/springo/framework/config"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DataSourcePoolProperties defines connection pool settings for the datasource.
type DataSourcePoolProperties struct {
	MaxOpenConns    int           `yaml:"max-open-conns"`
	MaxIdleConns    int           `yaml:"max-idle-conns"`
	ConnMaxLifetime time.Duration `yaml:"conn-max-lifetime"`
	ConnMaxIdleTime time.Duration `yaml:"conn-max-idle-time"`
	ConnTimeout     time.Duration `yaml:"conn-timeout"`
}

// Validate checks that connection pool parameters are non-negative.
func (p *DataSourcePoolProperties) Validate() error {
	if p.MaxOpenConns < 0 {
		return fmt.Errorf("pool max-open-conns must be non-negative: %d", p.MaxOpenConns)
	}
	if p.MaxIdleConns < 0 {
		return fmt.Errorf("pool max-idle-conns must be non-negative: %d", p.MaxIdleConns)
	}
	if p.ConnMaxLifetime < 0 {
		return fmt.Errorf("pool conn-max-lifetime must be non-negative: %v", p.ConnMaxLifetime)
	}
	if p.ConnMaxIdleTime < 0 {
		return fmt.Errorf("pool conn-max-idle-time must be non-negative: %v", p.ConnMaxIdleTime)
	}
	if p.ConnTimeout < 0 {
		return fmt.Errorf("pool conn-timeout must be non-negative: %v", p.ConnTimeout)
	}
	return nil
}

// DataSourceSessionProperties defines session-level initialization settings.
type DataSourceSessionProperties struct {
	InitSQL string `yaml:"init-sql"`
}

// Validate validates session-level settings.
func (p *DataSourceSessionProperties) Validate() error {
	return nil
}

// DataSourceMigrationProperties defines configuration for SQL file discovery, baseline, and migration table.
type DataSourceMigrationProperties struct {
	Table             string        `yaml:"table"`               // custom control table name (default: "springo_migrations")
	LockTimeout       time.Duration `yaml:"lock-timeout"`        // cluster lock timeout (default: 5m)
	Locations         []string      `yaml:"locations"`           // directories to scan for SQL migration files
	SQLPrefix         string        `yaml:"sql-prefix"`          // prefix for versioned SQL migrations (default: "V")
	BaselineOnMigrate bool          `yaml:"baseline-on-migrate"` // if true, marks migrations <= baseline-version as applied
	BaselineVersion   string        `yaml:"baseline-version"`    // baseline threshold version (default: "0")
	OutOfOrder        bool          `yaml:"out-of-order"`        // whether to allow out-of-order migrations
}

// Validate validates migration configuration parameters.
func (p *DataSourceMigrationProperties) Validate() error {
	if p.LockTimeout < 0 {
		return fmt.Errorf("migration lock-timeout must be non-negative: %v", p.LockTimeout)
	}
	return nil
}

// DataSourceProperties defines the database configuration in application.yaml.
type DataSourceProperties struct {
	Driver               string                        `yaml:"driver"`                 // sqlite, mysql, postgres
	Url                  string                        `yaml:"url"`                    // connection string or file path
	AutoMigrate          bool                          `yaml:"auto-migrate"`           // whether to run migrations on startup
	MigrationTable       string                        `yaml:"migration-table"`        // custom name for the control table (legacy)
	MigrationLockTimeout time.Duration                 `yaml:"migration-lock-timeout"` // duration like 5m (legacy)
	HealthCheck          bool                          `yaml:"health-check"`           // opt-in for health monitoring
	Pool                 DataSourcePoolProperties      `yaml:"pool"`
	Session              DataSourceSessionProperties   `yaml:"session"`
	Migration            DataSourceMigrationProperties `yaml:"migration"`
}

// Validate verifies database connection properties and delegates to sub-structures.
func (p *DataSourceProperties) Validate() error {
	if err := p.Pool.Validate(); err != nil {
		return fmt.Errorf("validating datasource pool properties: %w", err)
	}
	if err := p.Session.Validate(); err != nil {
		return fmt.Errorf("validating datasource session properties: %w", err)
	}
	if err := p.Migration.Validate(); err != nil {
		return fmt.Errorf("validating datasource migration properties: %w", err)
	}
	return nil
}

// GetMigrationTableName returns the configured migration control table name with fallbacks.
func (p *DataSourceProperties) GetMigrationTableName() string {
	if p.Migration.Table != "" {
		return p.Migration.Table
	}
	if p.MigrationTable != "" {
		return p.MigrationTable
	}
	return "springo_migrations"
}

// GetMigrationLockTimeout returns the configured migration lock timeout with fallbacks.
func (p *DataSourceProperties) GetMigrationLockTimeout() time.Duration {
	if p.Migration.LockTimeout > 0 {
		return p.Migration.LockTimeout
	}
	if p.MigrationLockTimeout > 0 {
		return p.MigrationLockTimeout
	}
	return 5 * time.Minute
}

// GetSQLPrefix returns the SQL migration filename prefix.
func (p *DataSourceProperties) GetSQLPrefix() string {
	if p.Migration.SQLPrefix != "" {
		return p.Migration.SQLPrefix
	}
	return "V"
}

// GetBaselineVersion returns the baseline cutoff version string.
func (p *DataSourceProperties) GetBaselineVersion() string {
	if p.Migration.BaselineVersion != "" {
		return p.Migration.BaselineVersion
	}
	return "0"
}

// AdditionalDataSources holds multiple named datasource configurations.
type AdditionalDataSources map[string]DataSourceProperties

func init() {
	// Register the primary properties under spring.datasource.
	config.RegisterProperties("spring.datasource", &DataSourceProperties{
		MigrationLockTimeout: 5 * time.Minute,
	})
	// Register additional datasources under spring.additional-datasources.
	config.RegisterProperties("spring.additional-datasources", &AdditionalDataSources{})
}

// Connect establishes a database connection based on properties.
func Connect(props *DataSourceProperties) (*gorm.DB, error) {
	if props == nil {
		return nil, fmt.Errorf("datasource properties not found")
	}

	dialector, err := createDialector(props)
	if err != nil {
		return nil, err
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	if err := configureConnection(db, props); err != nil {
		return nil, err
	}

	return db, nil
}

func createDialector(props *DataSourceProperties) (gorm.Dialector, error) {
	switch props.Driver {
	case "sqlite":
		return sqlite.Open(props.Url), nil
	case "mysql":
		return mysql.Open(props.Url), nil
	case "postgres":
		return postgres.Open(props.Url), nil
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", props.Driver)
	}
}

func configureConnection(db *gorm.DB, props *DataSourceProperties) error {
	if props.Driver == "sqlite" {
		applySQLitePragmas(db)
	}

	if err := configurePool(db, props); err != nil {
		return err
	}

	return executeInitSQL(db, props.Session.InitSQL)
}

func applySQLitePragmas(db *gorm.DB) {
	// Apply WAL mode and busy timeout pragmas for concurrent writes safety.
	db.Exec("PRAGMA journal_mode=WAL;")
	db.Exec("PRAGMA busy_timeout=5000;")
	db.Exec("PRAGMA synchronous=NORMAL;")
}

type poolSettings struct {
	maxOpen  int
	maxIdle  int
	lifetime time.Duration
	idleTime time.Duration
}

func resolvePoolSettings(props *DataSourceProperties) poolSettings {
	settings := poolSettings{
		maxOpen:  props.Pool.MaxOpenConns,
		maxIdle:  props.Pool.MaxIdleConns,
		lifetime: props.Pool.ConnMaxLifetime,
		idleTime: props.Pool.ConnMaxIdleTime,
	}

	if props.Driver == "sqlite" {
		resolveSQLiteDefaults(&settings, props.Url)
	} else {
		resolveStandardDefaults(&settings)
	}

	clampIdleConnections(&settings)
	return settings
}

func resolveSQLiteDefaults(settings *poolSettings, url string) {
	isMemory := strings.Contains(url, ":memory:") || strings.Contains(url, "mode=memory")
	if settings.maxOpen <= 0 {
		if isMemory {
			settings.maxOpen = 1
		} else {
			settings.maxOpen = 10
		}
	}
	if settings.maxIdle <= 0 {
		if isMemory {
			settings.maxIdle = 1
		} else {
			settings.maxIdle = 5
		}
	}
}

func resolveStandardDefaults(settings *poolSettings) {
	if settings.maxOpen <= 0 {
		settings.maxOpen = 25
	}
	if settings.maxIdle <= 0 {
		settings.maxIdle = 10
	}
	if settings.lifetime <= 0 {
		settings.lifetime = 30 * time.Minute
	}
}

func clampIdleConnections(settings *poolSettings) {
	if settings.maxOpen > 0 && settings.maxIdle > settings.maxOpen {
		slog.Warn(
			"datasource pool max-idle-conns exceeds max-open-conns, clamping to max-open-conns",
			"max_idle", settings.maxIdle,
			"max_open", settings.maxOpen,
		)
		settings.maxIdle = settings.maxOpen
	}
}

func configurePool(db *gorm.DB, props *DataSourceProperties) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}

	settings := resolvePoolSettings(props)
	sqlDB.SetMaxOpenConns(settings.maxOpen)
	sqlDB.SetMaxIdleConns(settings.maxIdle)

	if settings.lifetime > 0 {
		sqlDB.SetConnMaxLifetime(settings.lifetime)
	}
	if settings.idleTime > 0 {
		sqlDB.SetConnMaxIdleTime(settings.idleTime)
	}
	return nil
}

func executeInitSQL(db *gorm.DB, initSQL string) error {
	trimmed := strings.TrimSpace(initSQL)
	if trimmed == "" {
		return nil
	}
	if err := db.Exec(trimmed).Error; err != nil {
		return fmt.Errorf("executing datasource session init-sql: %w", err)
	}
	return nil
}
