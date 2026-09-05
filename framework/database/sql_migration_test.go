package database

import (
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDiscoverSQLMigrations_NaturalOrderAndExecution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "springo_sql_migrations_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	v1Content := "CREATE TABLE users_v1 (id INTEGER PRIMARY KEY, name TEXT);"
	v21Content := "CREATE TABLE users_v2_1 (id INTEGER PRIMARY KEY, email TEXT);"
	v10Content := "CREATE TABLE users_v10 (id INTEGER PRIMARY KEY, note TEXT);"

	_ = os.WriteFile(filepath.Join(tempDir, "V1__create_users.sql"), []byte(v1Content), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "V2.1__add_users_21.sql"), []byte(v21Content), 0644)
	_ = os.WriteFile(filepath.Join(tempDir, "V10__add_users_10.sql"), []byte(v10Content), 0644)

	discovered, err := DiscoverSQLMigrations([]string{tempDir}, "V")
	if err != nil {
		t.Fatalf("failed to discover migrations: %v", err)
	}
	if len(discovered) != 3 {
		t.Fatalf("expected 3 migrations, got %d", len(discovered))
	}

	if discovered[0].Name != "V1__create_users" ||
		discovered[1].Name != "V2.1__add_users_21" ||
		discovered[2].Name != "V10__add_users_10" {
		t.Fatalf("unexpected natural sort order: %#v", discovered)
	}

	db, err := gorm.Open(sqlite.Open("file:mem_sql_order?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	// Reset registration queue for test isolation.
	oldRegistered := registeredMigrations
	registeredMigrations = discovered
	t.Cleanup(func() {
		registeredMigrations = oldRegistered
	})

	mgr := NewMigrationManager(db, true)
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	var count int64
	if err := db.Table("users_v1").Count(&count).Error; err != nil {
		t.Fatalf("table users_v1 does not exist: %v", err)
	}
	if err := db.Table("users_v2_1").Count(&count).Error; err != nil {
		t.Fatalf("table users_v2_1 does not exist: %v", err)
	}
	if err := db.Table("users_v10").Count(&count).Error; err != nil {
		t.Fatalf("table users_v10 does not exist: %v", err)
	}
}

func TestSQLMigration_ChecksumMismatch(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "springo_checksum_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	sqlFile := filepath.Join(tempDir, "V1__create_items.sql")
	_ = os.WriteFile(sqlFile, []byte("CREATE TABLE items (id INTEGER PRIMARY KEY);"), 0644)

	discovered, err := DiscoverSQLMigrations([]string{tempDir}, "V")
	if err != nil {
		t.Fatalf("failed to discover migrations: %v", err)
	}

	db, err := gorm.Open(sqlite.Open("file:mem_sql_checksum?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	oldRegistered := registeredMigrations
	registeredMigrations = discovered
	t.Cleanup(func() {
		registeredMigrations = oldRegistered
	})

	mgr := NewMigrationManager(db, true)
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}

	// Tamper with the SQL file on disk after execution.
	_ = os.WriteFile(sqlFile, []byte("CREATE TABLE items (id INTEGER PRIMARY KEY, tampered TEXT);"), 0644)

	tamperedDiscovered, err := DiscoverSQLMigrations([]string{tempDir}, "V")
	if err != nil {
		t.Fatalf("failed to rediscover migrations: %v", err)
	}
	registeredMigrations = tamperedDiscovered

	err = mgr.Migrate()
	if err == nil {
		t.Fatalf("expected checksum integrity error, got nil")
	}
}

func TestSQLMigration_BaselineOnMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_sql_baseline?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	// Pre-create existing tables in DB.
	_ = db.Exec("CREATE TABLE legacy_users (id INTEGER PRIMARY KEY, name TEXT);").Error

	oldRegistered := registeredMigrations
	registeredMigrations = []Migration{
		{
			Name: "V1__create_legacy_users",
			Up: func(tx *gorm.DB) error {
				return tx.Exec("CREATE TABLE legacy_users (id INTEGER PRIMARY KEY, name TEXT);").Error
			},
			Checksum: "sum-v1",
		},
		{
			Name: "V2__create_legacy_roles",
			Up: func(tx *gorm.DB) error {
				return tx.Exec("CREATE TABLE legacy_roles (id INTEGER PRIMARY KEY, title TEXT);").Error
			},
			Checksum: "sum-v2",
		},
		{
			Name: "V3__create_new_feature",
			Up: func(tx *gorm.DB) error {
				return tx.Exec("CREATE TABLE feature_v3 (id INTEGER PRIMARY KEY, enabled BOOLEAN);").Error
			},
			Checksum: "sum-v3",
		},
	}
	t.Cleanup(func() {
		registeredMigrations = oldRegistered
	})

	props := &DataSourceProperties{
		Migration: DataSourceMigrationProperties{
			BaselineOnMigrate: true,
			BaselineVersion:   "V2",
		},
	}
	mgr := NewMigrationManagerWithProps(db, props, true)
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("baseline migration failed: %v", err)
	}

	// V1 and V2 should be marked in DB, V3 created.
	var count int64
	if err := db.Table("feature_v3").Count(&count).Error; err != nil {
		t.Fatalf("table feature_v3 should have been migrated: %v", err)
	}

	statusList, err := mgr.GetStatus()
	if err != nil {
		t.Fatalf("failed to get migration status: %v", err)
	}
	if len(statusList) != 3 {
		t.Fatalf("expected 3 status records, got %d", len(statusList))
	}
	for _, s := range statusList {
		if !s.Executed {
			t.Fatalf("expected migration %s to be marked as executed", s.Name)
		}
	}
}

func TestSQLMigration_OutOfOrderRejection(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_sql_outoforder?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	oldRegistered := registeredMigrations
	// Step 1: Run V2 first.
	registeredMigrations = []Migration{
		{
			Name: "V2__create_orders",
			Up: func(tx *gorm.DB) error {
				return tx.Exec("CREATE TABLE orders (id INTEGER PRIMARY KEY);").Error
			},
			Checksum: "sum-v2",
		},
	}
	t.Cleanup(func() {
		registeredMigrations = oldRegistered
	})

	mgr := NewMigrationManager(db, true)
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("failed to migrate V2: %v", err)
	}

	// Step 2: Introduce older V1 migration with out-of-order = false (default).
	registeredMigrations = append(registeredMigrations, Migration{
		Name: "V1__create_customers",
		Up: func(tx *gorm.DB) error {
			return tx.Exec("CREATE TABLE customers (id INTEGER PRIMARY KEY);").Error
		},
		Checksum: "sum-v1",
	})

	err = mgr.Migrate()
	if err == nil {
		t.Fatalf("expected out-of-order error when out-of-order is false")
	}
}

func TestSQLMigration_CoexistenceWithGoMigrations(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "springo_coexist_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	_ = os.WriteFile(
		filepath.Join(tempDir, "V1__init_schema.sql"),
		[]byte("CREATE TABLE coexist_schema (id INTEGER PRIMARY KEY);"),
		0644,
	)

	discovered, err := DiscoverSQLMigrations([]string{tempDir}, "V")
	if err != nil {
		t.Fatalf("failed to discover sql migrations: %v", err)
	}

	db, err := gorm.Open(sqlite.Open("file:mem_sql_coexist?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	oldRegistered := registeredMigrations
	// Combine SQL migration V1 with Go migration dated later.
	registeredMigrations = append([]Migration{}, discovered...)
	registeredMigrations = append(registeredMigrations, Migration{
		Name: "20260904_000001_seed_coexist",
		Up: func(tx *gorm.DB) error {
			return tx.Exec("INSERT INTO coexist_schema (id) VALUES (100);").Error
		},
		Checksum: "sum-go-seed",
	})
	t.Cleanup(func() {
		registeredMigrations = oldRegistered
	})

	mgr := NewMigrationManager(db, true)
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("migration failed for coexisting migrations: %v", err)
	}

	var count int64
	_ = db.Table("coexist_schema").Count(&count)
	if count != 1 {
		t.Fatalf("expected 1 row in coexist_schema, got %d", count)
	}
}

func TestSQLMigration_UndoScriptExecution(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "springo_undo_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(tempDir)
	})

	_ = os.WriteFile(
		filepath.Join(tempDir, "V1__create_products.sql"),
		[]byte("CREATE TABLE products_undo_test (id INTEGER PRIMARY KEY, sku TEXT);"),
		0644,
	)
	_ = os.WriteFile(
		filepath.Join(tempDir, "V1__create_products.undo.sql"),
		[]byte("DROP TABLE products_undo_test;"),
		0644,
	)

	discovered, err := DiscoverSQLMigrations([]string{tempDir}, "V")
	if err != nil {
		t.Fatalf("failed to discover migrations: %v", err)
	}
	if len(discovered) != 1 {
		t.Fatalf("expected 1 forward migration, got %d", len(discovered))
	}
	if discovered[0].Down == nil {
		t.Fatalf("expected Down function to be set from .undo.sql")
	}

	db, err := gorm.Open(sqlite.Open("file:mem_sql_undo?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("failed to open sqlite: %v", err)
	}
	sqlDB, _ := db.DB()
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	oldRegistered := registeredMigrations
	registeredMigrations = discovered
	t.Cleanup(func() {
		registeredMigrations = oldRegistered
	})

	mgr := NewMigrationManager(db, true)
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	// Rollback the migration.
	if err := mgr.Rollback(1); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Table should now be dropped.
	var count int64
	err = db.Table("products_undo_test").Count(&count).Error
	if err == nil {
		t.Fatalf("expected error querying dropped table products_undo_test")
	}
}
