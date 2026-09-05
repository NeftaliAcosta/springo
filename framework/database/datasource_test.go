package database

import (
	"os"
	"testing"
	"time"
)

func TestConnectSQLiteDefaultPoolSettings(t *testing.T) {
	// In-memory SQLite defaults: maxOpen = 1.
	memProps := &DataSourceProperties{
		Driver: "sqlite",
		Url:    ":memory:",
	}
	memDB, err := Connect(memProps)
	if err != nil {
		t.Fatalf("expected connect memory sqlite, got err: %v", err)
	}
	memSQL, err := memDB.DB()
	if err != nil {
		t.Fatalf("expected sql.DB from gorm, got err: %v", err)
	}
	t.Cleanup(func() {
		_ = memSQL.Close()
	})

	if memSQL.Stats().MaxOpenConnections != 1 {
		t.Fatalf("expected max open 1 for memory sqlite, got %d", memSQL.Stats().MaxOpenConnections)
	}

	// File-based SQLite defaults: maxOpen = 10.
	dbFile := "test_pool_default.db"
	_ = os.Remove(dbFile)
	t.Cleanup(func() {
		_ = os.Remove(dbFile)
	})

	fileProps := &DataSourceProperties{
		Driver: "sqlite",
		Url:    dbFile,
	}
	fileDB, err := Connect(fileProps)
	if err != nil {
		t.Fatalf("expected connect file sqlite, got err: %v", err)
	}
	fileSQL, err := fileDB.DB()
	if err != nil {
		t.Fatalf("expected sql.DB from file gorm, got err: %v", err)
	}
	t.Cleanup(func() {
		_ = fileSQL.Close()
	})

	if fileSQL.Stats().MaxOpenConnections != 10 {
		t.Fatalf("expected max open 10 for file sqlite, got %d", fileSQL.Stats().MaxOpenConnections)
	}
}

func TestConnectCustomPoolSettings(t *testing.T) {
	dbFile := "test_pool_custom.db"
	_ = os.Remove(dbFile)
	t.Cleanup(func() {
		_ = os.Remove(dbFile)
	})

	props := &DataSourceProperties{
		Driver: "sqlite",
		Url:    dbFile,
		Pool: DataSourcePoolProperties{
			MaxOpenConns:    7,
			MaxIdleConns:    3,
			ConnMaxLifetime: 15 * time.Minute,
			ConnMaxIdleTime: 5 * time.Minute,
		},
	}

	db, err := Connect(props)
	if err != nil {
		t.Fatalf("expected connect with custom pool, got err: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("expected sql.DB, got err: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if sqlDB.Stats().MaxOpenConnections != 7 {
		t.Fatalf("expected max open 7, got %d", sqlDB.Stats().MaxOpenConnections)
	}
}

func TestConnectClampingMaxIdleConns(t *testing.T) {
	dbFile := "test_pool_clamp.db"
	_ = os.Remove(dbFile)
	t.Cleanup(func() {
		_ = os.Remove(dbFile)
	})

	props := &DataSourceProperties{
		Driver: "sqlite",
		Url:    dbFile,
		Pool: DataSourcePoolProperties{
			MaxOpenConns: 4,
			MaxIdleConns: 12,
		},
	}

	db, err := Connect(props)
	if err != nil {
		t.Fatalf("expected connect with clamped idle conns, got err: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("expected sql.DB, got err: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	if sqlDB.Stats().MaxOpenConnections != 4 {
		t.Fatalf("expected max open 4, got %d", sqlDB.Stats().MaxOpenConnections)
	}
}

func TestConnectSessionInitSQL(t *testing.T) {
	props := &DataSourceProperties{
		Driver: "sqlite",
		Url:    ":memory:",
		Session: DataSourceSessionProperties{
			InitSQL: "CREATE TABLE session_init_check (id INTEGER PRIMARY KEY, marker TEXT);",
		},
	}

	db, err := Connect(props)
	if err != nil {
		t.Fatalf("expected connect with init-sql, got err: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	// Verify table was created by init-sql.
	var count int64
	err = db.Table("session_init_check").Count(&count).Error
	if err != nil {
		t.Fatalf("expected session_init_check table to exist, got err: %v", err)
	}
}

func TestConnectSessionInitSQLError(t *testing.T) {
	props := &DataSourceProperties{
		Driver: "sqlite",
		Url:    ":memory:",
		Session: DataSourceSessionProperties{
			InitSQL: "SYNTAX ERROR INVALID SQL;",
		},
	}

	_, err := Connect(props)
	if err == nil {
		t.Fatalf("expected error from invalid init-sql")
	}
}

func TestDataSourcePropertiesValidate(t *testing.T) {
	validProps := &DataSourceProperties{
		Driver: "sqlite",
		Url:    ":memory:",
		Pool: DataSourcePoolProperties{
			MaxOpenConns:    10,
			MaxIdleConns:    5,
			ConnMaxLifetime: 10 * time.Minute,
			ConnMaxIdleTime: 2 * time.Minute,
			ConnTimeout:     30 * time.Second,
		},
	}
	if err := validProps.Validate(); err != nil {
		t.Fatalf("expected valid properties, got err: %v", err)
	}

	invalidPoolCases := []struct {
		name  string
		props DataSourceProperties
	}{
		{
			name: "negative max open",
			props: DataSourceProperties{
				Pool: DataSourcePoolProperties{MaxOpenConns: -1},
			},
		},
		{
			name: "negative max idle",
			props: DataSourceProperties{
				Pool: DataSourcePoolProperties{MaxIdleConns: -1},
			},
		},
		{
			name: "negative conn max lifetime",
			props: DataSourceProperties{
				Pool: DataSourcePoolProperties{ConnMaxLifetime: -1 * time.Minute},
			},
		},
		{
			name: "negative conn max idle time",
			props: DataSourceProperties{
				Pool: DataSourcePoolProperties{ConnMaxIdleTime: -1 * time.Minute},
			},
		},
		{
			name: "negative conn timeout",
			props: DataSourceProperties{
				Pool: DataSourcePoolProperties{ConnTimeout: -1 * time.Second},
			},
		},
		{
			name: "negative migration lock timeout",
			props: DataSourceProperties{
				Migration: DataSourceMigrationProperties{LockTimeout: -1 * time.Second},
			},
		},
	}

	for _, tc := range invalidPoolCases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.props.Validate(); err == nil {
				t.Fatalf("expected validation error for %s", tc.name)
			}
		})
	}
}

func TestConnectEdgeCases(t *testing.T) {
	_, err := Connect(nil)
	if err == nil {
		t.Fatalf("expected error on nil properties")
	}

	_, err = Connect(&DataSourceProperties{Driver: "unsupported_db", Url: "localhost"})
	if err == nil {
		t.Fatalf("expected error on unsupported driver")
	}
}
