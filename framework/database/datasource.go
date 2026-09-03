package database

import (
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/config"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// DataSourceProperties defines the database configuration in application.yaml
type DataSourceProperties struct {
	Driver               string        `yaml:"driver"`                 // sqlite, mysql, postgres
	Url                  string        `yaml:"url"`                    // connection string or file path
	AutoMigrate          bool          `yaml:"auto-migrate"`           // whether to run migrations on startup
	MigrationTable       string        `yaml:"migration-table"`        // custom name for the control table
	MigrationLockTimeout time.Duration `yaml:"migration-lock-timeout"` // duration like 5m
	HealthCheck          bool          `yaml:"health-check"`           // opt-in for health monitoring
}

// AdditionalDataSources holds multiple named datasource configurations
type AdditionalDataSources map[string]DataSourceProperties

func init() {
	// Register the primary properties under spring.datasource
	config.RegisterProperties("spring.datasource", &DataSourceProperties{
		MigrationLockTimeout: 5 * time.Minute,
	})
	// Register additional datasources under spring.additional-datasources
	config.RegisterProperties("spring.additional-datasources", &AdditionalDataSources{})
}

// Connect establishes a database connection based on properties
func Connect(props *DataSourceProperties) (*gorm.DB, error) {
	if props == nil {
		return nil, fmt.Errorf("datasource properties not found")
	}

	var dialector gorm.Dialector

	switch props.Driver {
	case "sqlite":
		dialector = sqlite.Open(props.Url)
	case "mysql":
		dialector = mysql.Open(props.Url)
	case "postgres":
		dialector = postgres.Open(props.Url)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s", props.Driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, err
	}

	switch props.Driver {
	case "sqlite":
		sqlDB, err := db.DB()
		if err == nil {
			// Apply WAL mode and busy timeout pragmas for concurrent writes safety
			db.Exec("PRAGMA journal_mode=WAL;")
			db.Exec("PRAGMA busy_timeout=5000;")
			db.Exec("PRAGMA synchronous=NORMAL;")

			// Handle in-memory vs file-based connection pooling
			if strings.Contains(props.Url, ":memory:") || strings.Contains(props.Url, "mode=memory") {
				sqlDB.SetMaxOpenConns(1)
			} else {
				sqlDB.SetMaxOpenConns(10)
				sqlDB.SetMaxIdleConns(5)
			}
		}
	case "postgres", "mysql":
		sqlDB, err := db.DB()
		if err == nil {
			sqlDB.SetMaxOpenConns(25)
			sqlDB.SetMaxIdleConns(10)
			sqlDB.SetConnMaxLifetime(30 * time.Minute)
		}
	}

	return db, nil
}
