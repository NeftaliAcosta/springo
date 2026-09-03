package database

import (
	"context"
	"os"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type MultiDBTestEntity struct {
	ID      uint   `gorm:"primaryKey"`
	Title   string `gorm:"size:150"`
	SprinGo string `springo:"audited" gorm:"-"`
}

// SetupMultiDBEntity initializes test tables and enables auditing for MultiDBTestEntity.
func setupMultiDBEntity(t *testing.T, db *gorm.DB) {
	err := db.AutoMigrate(&MultiDBTestEntity{})
	if err != nil {
		t.Fatalf("Failed to auto-migrate MultiDBTestEntity: %v", err)
	}

	err = EnableAuditing(db, &MultiDBTestEntity{})
	if err != nil {
		t.Fatalf("Failed to enable auditing on MultiDBTestEntity: %v", err)
	}
}

// TestMultiDB_AuditingAndMigration_SQLite verifies auditing and migrations on SQLite.
func TestMultiDB_AuditingAndMigration_SQLite(t *testing.T) {
	dbFile := "test_multidb.db"
	_ = os.Remove(dbFile)

	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
		_ = os.Remove(dbFile)
	})

	setupMultiDBEntity(t, db)

	ctx := context.WithValue(context.Background(), "username", "multidb_user") //nolint:staticcheck // Framework audit uses string context keys
	entity := &MultiDBTestEntity{Title: "MultiDB Integration Test"}

	err = db.WithContext(ctx).Create(entity).Error
	if err != nil {
		t.Fatalf("Failed to insert entity in SQLite: %v", err)
	}

	var auditCount int64
	db.Table("multi_db_test_entities_aud").Count(&auditCount)
	if auditCount != 1 {
		t.Errorf("Expected 1 audit record in SQLite, got %d", auditCount)
	}
}

// TestMultiDB_DDLGeneration_Dialects validates DDL output for Postgres, MySQL, and SQLite.
func TestMultiDB_DDLGeneration_Dialects(t *testing.T) {
	dialects := []struct {
		name         string
		expectedPK   string
		expectedTS   string
		quotedTable  string
		quotedColumn string
	}{
		{
			name:         "postgres",
			expectedPK:   `"audit_id" BIGSERIAL PRIMARY KEY`,
			expectedTS:   "TIMESTAMPTZ",
			quotedTable:  `"items_aud"`,
			quotedColumn: `"title"`,
		},
		{
			name:         "mysql",
			expectedPK:   "`audit_id` INT AUTO_INCREMENT PRIMARY KEY",
			expectedTS:   "DATETIME",
			quotedTable:  "`items_aud`",
			quotedColumn: "`title`",
		},
		{
			name:         "sqlite",
			expectedPK:   "audit_id INTEGER PRIMARY KEY AUTOINCREMENT",
			expectedTS:   "DATETIME",
			quotedTable:  "items_aud",
			quotedColumn: "title",
		},
	}

	for _, d := range dialects {
		t.Run("DDL dialect "+d.name, func(t *testing.T) {
			pk := getAuditPKDef(d.name)
			if pk != d.expectedPK {
				t.Errorf("dialect %s: expected PK '%s', got '%s'", d.name, d.expectedPK, pk)
			}

			ts := getAuditTimestampType(d.name)
			if ts != d.expectedTS {
				t.Errorf("dialect %s: expected TS '%s', got '%s'", d.name, d.expectedTS, ts)
			}

			tbl := quoteIdentifier(d.name, "items_aud")
			if tbl != d.quotedTable {
				t.Errorf("dialect %s: expected quoted table '%s', got '%s'", d.name, d.quotedTable, tbl)
			}

			col := quoteIdentifier(d.name, "title")
			if col != d.quotedColumn {
				t.Errorf("dialect %s: expected quoted column '%s', got '%s'", d.name, d.quotedColumn, col)
			}
		})
	}
}

// TestMultiDB_PostgreSQL_Integration executes integration tests when TEST_POSTGRES_URL is set.
func TestMultiDB_PostgreSQL_Integration(t *testing.T) {
	pgUrl := os.Getenv("TEST_POSTGRES_URL")
	if pgUrl == "" {
		t.Skip("Skipping PostgreSQL integration test: TEST_POSTGRES_URL environment variable not set")
	}

	props := &DataSourceProperties{
		Driver: "postgres",
		Url:    pgUrl,
	}

	db, err := Connect(props)
	if err != nil {
		t.Fatalf("Failed to connect to PostgreSQL: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	setupMultiDBEntity(t, db)

	ctx := context.WithValue(context.Background(), "username", "pg_audit_user") //nolint:staticcheck // Framework audit uses string context keys
	entity := &MultiDBTestEntity{Title: "Postgres Test"}

	err = db.WithContext(ctx).Create(entity).Error
	if err != nil {
		t.Fatalf("Failed to insert record in PostgreSQL: %v", err)
	}
}

// TestMultiDB_MySQL_Integration executes integration tests when TEST_MYSQL_URL is set.
func TestMultiDB_MySQL_Integration(t *testing.T) {
	mysqlUrl := os.Getenv("TEST_MYSQL_URL")
	if mysqlUrl == "" {
		t.Skip("Skipping MySQL integration test: TEST_MYSQL_URL environment variable not set")
	}

	props := &DataSourceProperties{
		Driver: "mysql",
		Url:    mysqlUrl,
	}

	db, err := Connect(props)
	if err != nil {
		t.Fatalf("Failed to connect to MySQL: %v", err)
	}

	t.Cleanup(func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})

	setupMultiDBEntity(t, db)

	ctx := context.WithValue(context.Background(), "username", "mysql_audit_user") //nolint:staticcheck // Framework audit uses string context keys
	entity := &MultiDBTestEntity{Title: "MySQL Test"}

	err = db.WithContext(ctx).Create(entity).Error
	if err != nil {
		t.Fatalf("Failed to insert record in MySQL: %v", err)
	}
}

// TestMultiDB_ConnectDataSource verifies Connect error handling and driver options.
func TestMultiDB_ConnectDataSource(t *testing.T) {
	t.Run("Unsupported driver returns error", func(t *testing.T) {
		_, err := Connect(&DataSourceProperties{Driver: "oracle", Url: "oracle://localhost"})
		if err == nil {
			t.Error("Expected error for unsupported driver 'oracle', got nil")
		}
		if !strings.Contains(err.Error(), "unsupported database driver") {
			t.Errorf("Expected error to mention 'unsupported database driver', got %v", err)
		}
	})

	t.Run("Nil properties returns error", func(t *testing.T) {
		_, err := Connect(nil)
		if err == nil {
			t.Error("Expected error for nil properties, got nil")
		}
	})

	t.Run("Valid SQLite in-memory connect succeeds", func(t *testing.T) {
		db, err := Connect(&DataSourceProperties{Driver: "sqlite", Url: ":memory:"})
		if err != nil {
			t.Fatalf("Expected successful SQLite connect, got %v", err)
		}
		if db == nil {
			t.Fatal("Expected non-nil *gorm.DB")
		}
	})
}
