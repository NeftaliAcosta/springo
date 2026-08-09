package database

import (
	"errors"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// TestMigration_SemanticSorting asserts that compareMigrationNames sorts versioned
// and timestamped migration names correctly, preventing pure alphabetical bugs.
func TestMigration_SemanticSorting(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool // true if a < b semantically
	}{
		// Version style
		{"V1_2__add_users", "V1_10__add_roles", true},
		{"V1.2__add_users", "V1.10__add_roles", true},
		{"V1.10__add_roles", "V1.2__add_users", false},
		{"V1_10_1__patch", "V1_10__add_roles", false},
		{"V2.0__new_major", "V1.99__old_major", false},

		// Timestamp style
		{"20260614_000001_create_users_table", "20260614_000002_add_roles", true},
		{"20260614000001_create_users", "20260614000002_add_roles", true},
		{"20260620_120000_update_schema", "20260614_000002_add_roles", false},

		// Alphabetical fallback
		{"add_users", "create_roles", true},
		{"create_roles", "add_users", false},
	}

	for _, tt := range tests {
		res := compareMigrationNames(tt.a, tt.b)
		if res != tt.expected {
			t.Errorf("compareMigrationNames(%q, %q) = %t; want %t", tt.a, tt.b, res, tt.expected)
		}
	}
}

// TestMigrationManager_Success verifies typical migration, batch incrementing, and rollback/reset flows.
func TestMigrationManager_Success(t *testing.T) {
	// Setup sqlite DB
	db, err := gorm.Open(sqlite.Open("file:mem_migrator?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	// Make sure we start with a clean slate
	db.Exec("DROP TABLE IF EXISTS springo_migrations")
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")
	db.Exec("DROP TABLE IF EXISTS migrate_test_users")

	// Set custom migrations table name
	customTableName = "springo_migrations_test_success"
	defer func() { customTableName = "springo_migrations" }()

	// Register test migrations in global registeredMigrations
	oldRegistered := registeredMigrations
	defer func() { registeredMigrations = oldRegistered }()

	// Reset registration queue
	registeredMigrations = []Migration{
		{
			Name: "V1.0__create_user_table",
			Up: func(db *gorm.DB) error {
				return db.Exec("CREATE TABLE migrate_test_users (id INTEGER PRIMARY KEY, name TEXT)").Error
			},
			Down: func(db *gorm.DB) error {
				return db.Exec("DROP TABLE migrate_test_users").Error
			},
			Checksum: "hash-v1.0",
		},
		{
			Name: "V1.1__add_initial_data",
			Up: func(db *gorm.DB) error {
				return db.Exec("INSERT INTO migrate_test_users (id, name) VALUES (1, 'Alice')").Error
			},
			Down: func(db *gorm.DB) error {
				return db.Exec("DELETE FROM migrate_test_users WHERE id = 1").Error
			},
			Checksum: "hash-v1.1",
		},
	}

	mgr := NewMigrationManager(db, true)

	// Run migration batch 1
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("Migration failed: %v", err)
	}

	// Verify table migrate_test_users exists and has 1 record
	var count int64
	if err := db.Table("migrate_test_users").Count(&count).Error; err != nil {
		t.Fatalf("Failed to query migrate_test_users: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 user, got %d", count)
	}

	// Verify springo_migrations table has two entries with batch 1
	var records []MigrationRecord
	if err := db.Table(customTableName).Find(&records).Error; err != nil {
		t.Fatalf("Failed to query migrations table: %v", err)
	}
	if len(records) != 2 {
		t.Errorf("Expected 2 migration records, got %d", len(records))
	}
	for _, r := range records {
		if r.Batch != 1 {
			t.Errorf("Expected migration batch 1, got %d", r.Batch)
		}
	}

	// Run migration again with no changes, should do nothing
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("Subsequent migration execution failed: %v", err)
	}

	// Add another migration and run as batch 2
	registeredMigrations = append(registeredMigrations, Migration{
		Name: "V1.2__add_bob",
		Up: func(db *gorm.DB) error {
			return db.Exec("INSERT INTO migrate_test_users (id, name) VALUES (2, 'Bob')").Error
		},
		Down: func(db *gorm.DB) error {
			return db.Exec("DELETE FROM migrate_test_users WHERE id = 2").Error
		},
		Checksum: "hash-v1.2",
	})

	if err := mgr.Migrate(); err != nil {
		t.Fatalf("Failed to run batch 2: %v", err)
	}

	// Verify Bob exists and records batch count
	var bobCount int64
	db.Table("migrate_test_users").Where("name = ?", "Bob").Count(&bobCount)
	if bobCount != 1 {
		t.Errorf("Expected Bob to exist, got count %d", bobCount)
	}

	var latestRecord MigrationRecord
	db.Table(customTableName).Order("batch desc").First(&latestRecord)
	if latestRecord.Batch != 2 {
		t.Errorf("Expected latest migration to be batch 2, got %d", latestRecord.Batch)
	}

	// Test Rollback of latest batch (batch 2: Bob)
	if err := mgr.Rollback(0); err != nil {
		t.Fatalf("Rollback failed: %v", err)
	}

	// Bob should be deleted, batch 2 should be gone
	db.Table("migrate_test_users").Where("name = ?", "Bob").Count(&bobCount)
	if bobCount != 0 {
		t.Errorf("Expected Bob to be rolled back and deleted, but still exists")
	}

	// Test Reset (reverts batch 1)
	if err := mgr.Reset(); err != nil {
		t.Fatalf("Reset failed: %v", err)
	}

	// Users table should be dropped completely
	err = db.Table("migrate_test_users").Count(&count).Error
	if err == nil {
		t.Error("Expected migrate_test_users table to be dropped, but table still exists")
	}
}

// TestMigrationManager_IntegrityChecksumViolation checks that modifying migration logic
// after execution raises a validation checksum error at start.
func TestMigrationManager_IntegrityChecksumViolation(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_migrator_chk?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS springo_migrations_test_chk")
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")

	customTableName = "springo_migrations_test_chk"
	defer func() { customTableName = "springo_migrations" }()

	oldRegistered := registeredMigrations
	defer func() { registeredMigrations = oldRegistered }()

	registeredMigrations = []Migration{
		{
			Name:     "V1.0__test_mig",
			Up:       func(db *gorm.DB) error { return nil },
			Checksum: "initial-hash",
		},
	}

	mgr := NewMigrationManager(db, true)

	// Apply migration
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("Failed initial migration: %v", err)
	}

	// Modify the registered migration's checksum in memory
	registeredMigrations[0].Checksum = "altered-hash"

	// Subsequent migration should fail due to checksum mismatch
	err = mgr.Migrate()
	if err == nil {
		t.Fatal("Expected integrity violation error, got nil")
	}
	if !strings.Contains(err.Error(), "integrity violation") {
		t.Errorf("Expected checksum validation error containing 'integrity violation', got: %v", err)
	}
	t.Logf("Successfully caught checksum violation: %v", err)
}

// TestMigrationManager_ClusterLock verifies concurrency locks in cluster setups.
func TestMigrationManager_ClusterLock(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_migrator_lock?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS springo_migrations_test_lock")
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")

	customTableName = "springo_migrations_test_lock"
	defer func() { customTableName = "springo_migrations" }()

	oldRegistered := registeredMigrations
	defer func() { registeredMigrations = oldRegistered }()

	registeredMigrations = []Migration{
		{
			Name: "V1.0__concurrency_test",
			Up: func(db *gorm.DB) error {
				// Artificial delay to simulate long migration
				time.Sleep(100 * time.Millisecond)
				return nil
			},
		},
	}

	mgr1 := NewMigrationManager(db, true)
	mgr2 := NewMigrationManager(db, true)

	// Launch migration in parallel
	var wg sync.WaitGroup
	wg.Add(2)

	var err1, err2 error

	go func() {
		defer wg.Done()
		err1 = mgr1.Migrate()
	}()

	go func() {
		defer wg.Done()
		// Small delay to ensure mgr1 starts first
		time.Sleep(20 * time.Millisecond)
		err2 = mgr2.Migrate()
	}()

	wg.Wait()

	// Both should succeed because the second one will wait for the lock to release,
	// then check GORM history and skip running, completing gracefully.
	if err1 != nil {
		t.Errorf("Manager 1 failed to migrate: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Manager 2 failed to migrate: %v", err2)
	}

	// Verify that the table lock was cleaned up
	var lock MigrationLock
	err = db.Where("lock_key = ?", "migration_lock").First(&lock).Error
	if err != nil {
		t.Fatalf("Failed to fetch lock record: %v", err)
	}
	if lock.Locked {
		t.Errorf("Expected lock to be released, but lock.Locked is true")
	}
}

func TestMigrationManager_Heartbeat(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_migrator_heartbeat?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS springo_migrations_test_hb")
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")

	customTableName = "springo_migrations_test_hb"
	defer func() { customTableName = "springo_migrations" }()

	oldRegistered := registeredMigrations
	defer func() { registeredMigrations = oldRegistered }()

	registeredMigrations = []Migration{
		{
			Name: "V1.0__slow_migration",
			Up: func(db *gorm.DB) error {
				return nil
			},
		},
	}

	mgr := NewMigrationManager(db, true)

	// Ensure table exists
	db.AutoMigrate(&MigrationLock{})
	lockedBy := "test-runner-123"
	initialLock := MigrationLock{
		LockKey:  "migration_lock",
		Locked:   true,
		LockedAt: time.Now().Add(-10 * time.Minute),
		LockedBy: lockedBy,
	}
	db.Create(&initialLock)

	// Call refreshLock
	mgr.refreshLock(db, lockedBy)

	// Assert: LockedAt should be updated to a very recent time
	var updatedLock MigrationLock
	db.Where("lock_key = ?", "migration_lock").First(&updatedLock)
	if time.Since(updatedLock.LockedAt) > 5*time.Second {
		t.Errorf("Expected LockedAt to be updated by refreshLock, got: %v", updatedLock.LockedAt)
	}
}

type controlledReader struct {
	bytes []byte
	err   error
}

func (c *controlledReader) Read(p []byte) (n int, err error) {
	if c.err != nil {
		return 0, c.err
	}
	copy(p, c.bytes)
	return len(p), nil
}

func TestMigrationManager_RandReadError(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_migrator_rand_err?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")

	mgr := NewMigrationManager(db, true)

	oldReader := entropySource
	entropySource = &controlledReader{err: errors.New("injected entropy failure")}
	defer func() {
		entropySource = oldReader
	}()

	err = mgr.Migrate()
	if err == nil {
		t.Fatal("Expected Migrate to fail when entropy source fails, but got nil")
	}
	if !strings.Contains(err.Error(), "injected entropy failure") {
		t.Errorf("Expected error to contain 'injected entropy failure', got: %v", err)
	}
}

func TestMigrationManager_DifferentIdentities(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_migrator_diff_ids?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	customTableName = "springo_migrations_test_diff_ids"
	defer func() { customTableName = "springo_migrations" }()
	db.Exec("DROP TABLE IF EXISTS springo_migrations_test_diff_ids")
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")

	oldRegistered := registeredMigrations
	defer func() { registeredMigrations = oldRegistered }()

	var capturedIdentities []string
	registeredMigrations = []Migration{
		{
			Name: "V1.0__inspect_lock",
			Up: func(db *gorm.DB) error {
				var lock MigrationLock
				if err := db.Where("lock_key = ?", "migration_lock").First(&lock).Error; err != nil {
					return err
				}
				capturedIdentities = append(capturedIdentities, lock.LockedBy)
				return nil
			},
		},
	}

	mgr := NewMigrationManager(db, true)

	// Run 1: Mock returning specific bytes
	oldReader := entropySource
	ctrlReader := &controlledReader{bytes: []byte{1, 2, 3, 4, 5, 6, 7, 8}}
	entropySource = ctrlReader
	defer func() {
		entropySource = oldReader
	}()

	if err := mgr.Migrate(); err != nil {
		t.Fatalf("First Migrate failed: %v", err)
	}

	// Clean database records to run again
	db.Exec("DROP TABLE IF EXISTS springo_migrations_test_diff_ids")
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")

	// Run 2: Mock returning different bytes
	ctrlReader.bytes = []byte{8, 7, 6, 5, 4, 3, 2, 1}
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("Second Migrate failed: %v", err)
	}

	if len(capturedIdentities) != 2 {
		t.Fatalf("Expected 2 captured identities, got %d", len(capturedIdentities))
	}

	if !strings.Contains(capturedIdentities[0], "0102030405060708") {
		t.Errorf("Expected first identity to contain unique token, got: %s", capturedIdentities[0])
	}
	if !strings.Contains(capturedIdentities[1], "0807060504030201") {
		t.Errorf("Expected second identity to contain unique token, got: %s", capturedIdentities[1])
	}
	if capturedIdentities[0] == capturedIdentities[1] {
		t.Errorf("Expected identities to be different, but they are identical: %s", capturedIdentities[0])
	}
}

func TestMigrationManager_HeartbeatDynamicInterval(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:mem_migrator_dynamic_hb?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	customTableName = "springo_migrations_test_dynamic_hb"
	defer func() { customTableName = "springo_migrations" }()
	db.Exec("DROP TABLE IF EXISTS springo_migrations_test_dynamic_hb")
	db.Exec("DROP TABLE IF EXISTS springo_migrations_lock")

	oldRegistered := registeredMigrations
	defer func() { registeredMigrations = oldRegistered }()

	// Register DataSourceProperties bean with a custom small timeout of 3s.
	// The derived heartbeat interval will be 3s / 3 = 1s.
	props := &DataSourceProperties{
		MigrationLockTimeout: 3 * time.Second,
	}
	ioc.GetContainer().RegisterBean("DataSourceProperties", props)
	defer ioc.GetContainer().RegisterBean("DataSourceProperties", nil)

	var refreshCount int
	var refreshMu sync.Mutex

	// Register callback to intercept updates on springo_migrations_lock and increment counter,
	// setting DryRun to true to bypass DB write locks.
	db.Callback().Update().Before("gorm:update").Register("intercept_hb", func(tx *gorm.DB) {
		if tx.Statement.Table == "springo_migrations_lock" || (tx.Statement.Schema != nil && tx.Statement.Schema.Table == "springo_migrations_lock") {
			isAcquireOrRelease := false
			if m, ok := tx.Statement.Dest.(map[string]interface{}); ok {
				if _, hasLocked := m["locked"]; hasLocked {
					isAcquireOrRelease = true
				}
			}
			if !isAcquireOrRelease {
				refreshMu.Lock()
				refreshCount++
				refreshMu.Unlock()
				tx.DryRun = true
			}
		}
	})

	registeredMigrations = []Migration{
		{
			Name: "V1.0__slow_migration_heartbeat",
			Up: func(db *gorm.DB) error {
				// Sleep for 2.5 seconds to guarantee that the 1-second heartbeat ticker fires at least twice
				time.Sleep(2500 * time.Millisecond)
				return nil
			},
		},
	}

	mgr := NewMigrationManager(db, true)
	if err := mgr.Migrate(); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	refreshMu.Lock()
	count := refreshCount
	refreshMu.Unlock()

	// We expect the 1s ticker to have fired at least 2 times during the 2.5s migration
	if count < 2 {
		t.Errorf("Expected at least 2 heartbeat refreshes, got %d", count)
	}
}

func TestMigrationManager_ResetAndRefreshPropagateQueryErrors(t *testing.T) {
	tests := []struct {
		name string
		run  func(*MigrationManager) error
	}{
		{name: "reset", run: func(m *MigrationManager) error { return m.Reset() }},
		{name: "refresh", run: func(m *MigrationManager) error { return m.Refresh() }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn := fmt.Sprintf("file:migrator_%s_query_error?mode=memory&cache=shared", tt.name)
			db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
			if err != nil {
				t.Fatalf("failed to open database: %v", err)
			}

			tableName := "springo_migrations_" + tt.name + "_query_error"
			restoreTable := getCustomTableName()
			SetMigrationTableName(tableName)
			t.Cleanup(func() { SetMigrationTableName(restoreTable) })

			if err := db.Table(tableName).AutoMigrate(&MigrationRecord{}); err != nil {
				t.Fatalf("failed to initialize migrations table: %v", err)
			}

			expectedErr := errors.New("forced migration query failure")
			callbackName := "force_" + tt.name + "_query_error"
			if err := db.Callback().Query().Before("gorm:query").Register(callbackName, func(tx *gorm.DB) {
				if tx.Statement.Table == tableName {
					tx.AddError(expectedErr)
				}
			}); err != nil {
				t.Fatalf("failed to register query callback: %v", err)
			}

			err = tt.run(NewMigrationManager(db, true))
			if !errors.Is(err, expectedErr) {
				t.Fatalf("expected query error to propagate, got %v", err)
			}
		})
	}
}
