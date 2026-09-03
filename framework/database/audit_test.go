package database

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type TestAuditedEntity struct {
	ID      uint   `gorm:"primaryKey"`
	Name    string `gorm:"size:100"`
	Email   string `gorm:"size:100"`
	SprinGo string `springo:"audited" gorm:"-"`
}

type TestNonAuditedEntity struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

type TestStrictEntity struct {
	ID      uint   `gorm:"primaryKey"`
	Name    string `gorm:"size:100"`
	SprinGo string `springo:"audited;user_key=company_email" gorm:"-"`
}

type TestStrictIntEntity struct {
	ID      uint   `gorm:"primaryKey"`
	Name    string `gorm:"size:100"`
	SprinGo string `springo:"audited;user_key=tenant_id" gorm:"-"`
}

// SetupAuditTestDB initializes an in-memory SQLite database with auditing enabled.
func setupAuditTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	err = db.AutoMigrate(&TestAuditedEntity{}, &TestNonAuditedEntity{}, &TestStrictEntity{}, &TestStrictIntEntity{})
	if err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	err = EnableAuditing(db, &TestAuditedEntity{}, &TestNonAuditedEntity{}, &TestStrictEntity{}, &TestStrictIntEntity{})
	if err != nil {
		t.Fatalf("Failed to enable auditing: %v", err)
	}

	if !db.Migrator().HasTable("test_audited_entities_aud") {
		t.Error("Expected table test_audited_entities_aud to be created")
	}
	if db.Migrator().HasTable("test_non_audited_entities_aud") {
		t.Error("Did not expect table test_non_audited_entities_aud to be created")
	}

	return db
}

func TestAuditing_CreateUpdateDelete(t *testing.T) {
	db := setupAuditTestDB(t)

	t.Run("Create INSERT audit record", func(t *testing.T) {
		testAuditInsert(t, db)
	})

	t.Run("Update UPDATE audit record", func(t *testing.T) {
		testAuditUpdate(t, db)
	})

	t.Run("Delete DELETE audit record with anonymous fallback", func(t *testing.T) {
		testAuditDelete(t, db)
	})

	t.Run("Non-audited entity produces no audit table", func(t *testing.T) {
		testNonAuditedEntity(t, db)
	})

	t.Run("Strict mode aborts on missing key", func(t *testing.T) {
		testStrictModeMissingKey(t, db)
	})

	t.Run("Strict mode succeeds on correct key", func(t *testing.T) {
		testStrictModeSuccess(t, db)
	})

	t.Run("Strict mode succeeds on non-string (int/float64) key", func(t *testing.T) {
		testStrictModeNonStringKey(t, db)
	})
}

// TestAuditInsert verifies audit record creation on INSERT.
func testAuditInsert(t *testing.T, db *gorm.DB) {
	ctx := context.WithValue(context.Background(), "username", "admin_user")
	entity := &TestAuditedEntity{Name: "Juan", Email: "juan@example.com"}

	err := db.WithContext(ctx).Create(entity).Error
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	var auditCount int64
	db.Table("test_audited_entities_aud").Count(&auditCount)
	if auditCount != 1 {
		t.Fatalf("Expected 1 audit record, got %d", auditCount)
	}

	var auditData map[string]interface{}
	db.Table("test_audited_entities_aud").Take(&auditData)

	if auditData["rev_type"] != "INSERT" {
		t.Errorf("Expected rev_type 'INSERT', got %v", auditData["rev_type"])
	}
	if auditData["rev_user"] != "admin_user" {
		t.Errorf("Expected rev_user 'admin_user', got %v", auditData["rev_user"])
	}
	if auditData["name"] != "Juan" {
		t.Errorf("Expected name 'Juan', got %v", auditData["name"])
	}
	if auditData["email"] != "juan@example.com" {
		t.Errorf("Expected email 'juan@example.com', got %v", auditData["email"])
	}
}

// TestAuditUpdate verifies audit record creation on UPDATE.
func testAuditUpdate(t *testing.T, db *gorm.DB) {
	ctx := context.WithValue(context.Background(), "username", "admin_user")
	var entity TestAuditedEntity
	db.First(&entity)

	entity.Name = "Juan Carlos"
	err := db.WithContext(ctx).Save(&entity).Error
	if err != nil {
		t.Fatalf("Failed to update record: %v", err)
	}

	var auditCount int64
	db.Table("test_audited_entities_aud").Count(&auditCount)
	if auditCount != 2 {
		t.Fatalf("Expected 2 audit records, got %d", auditCount)
	}

	var latestAuditData map[string]interface{}
	db.Table("test_audited_entities_aud").Order("audit_id desc").Take(&latestAuditData)

	if latestAuditData["rev_type"] != "UPDATE" {
		t.Errorf("Expected rev_type 'UPDATE', got %v", latestAuditData["rev_type"])
	}
	if latestAuditData["name"] != "Juan Carlos" {
		t.Errorf("Expected name 'Juan Carlos', got %v", latestAuditData["name"])
	}
}

// TestAuditDelete verifies audit record creation on DELETE.
func testAuditDelete(t *testing.T, db *gorm.DB) {
	var entity TestAuditedEntity
	db.First(&entity)

	err := db.Delete(&entity).Error
	if err != nil {
		t.Fatalf("Failed to delete record: %v", err)
	}

	var auditCount int64
	db.Table("test_audited_entities_aud").Count(&auditCount)
	if auditCount != 3 {
		t.Fatalf("Expected 3 audit records, got %d", auditCount)
	}

	var deleteAuditData map[string]interface{}
	db.Table("test_audited_entities_aud").Order("audit_id desc").Take(&deleteAuditData)

	if deleteAuditData["rev_type"] != "DELETE" {
		t.Errorf("Expected rev_type 'DELETE', got %v", deleteAuditData["rev_type"])
	}
	if deleteAuditData["rev_user"] != "anonymous" {
		t.Errorf("Expected fallback rev_user 'anonymous', got %v", deleteAuditData["rev_user"])
	}
}

// TestNonAuditedEntity verifies non-audited entities do not produce audit records.
func testNonAuditedEntity(t *testing.T, db *gorm.DB) {
	nonAudited := &TestNonAuditedEntity{Name: "SkipMe"}
	err := db.Create(nonAudited).Error
	if err != nil {
		t.Fatalf("Failed to create non-audited record: %v", err)
	}

	if db.Migrator().HasTable("test_non_audited_entities_aud") {
		t.Error("Non-audited entity should not have an audit table")
	}
}

// TestStrictModeMissingKey verifies strict user_key mode aborts when required context key is missing.
func testStrictModeMissingKey(t *testing.T, db *gorm.DB) {
	strictEntity := &TestStrictEntity{Name: "Strict No Key"}
	ctxEmpty := context.Background()

	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctxEmpty).Create(strictEntity).Error
	})

	if err == nil {
		t.Error("Expected error when saving strict entity without required context key, but got nil")
	}

	var count int64
	db.Model(&TestStrictEntity{}).Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 records in DB after strict rollback, got %d", count)
	}
}

// TestStrictModeSuccess verifies strict user_key mode succeeds when context key is present.
func testStrictModeSuccess(t *testing.T, db *gorm.DB) {
	strictEntity := &TestStrictEntity{Name: "Strict With Key"}
	ctxWithKey := context.WithValue(context.Background(), "company_email", "admin@acme.com")

	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctxWithKey).Create(strictEntity).Error
	})

	if err != nil {
		t.Fatalf("Expected successful save for strict entity, got error: %v", err)
	}

	var count int64
	db.Model(&TestStrictEntity{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 record in DB, got %d", count)
	}

	var auditData map[string]interface{}
	db.Table("test_strict_entities_aud").Take(&auditData)
	if auditData["rev_user"] != "admin@acme.com" {
		t.Errorf("Expected rev_user 'admin@acme.com', got %v", auditData["rev_user"])
	}
}

// TestStrictModeNonStringKey verifies strict mode works with non-string keys like int.
func testStrictModeNonStringKey(t *testing.T, db *gorm.DB) {
	strictEntity := &TestStrictIntEntity{Name: "Strict With Int Key"}
	ctxWithKey := context.WithValue(context.Background(), "tenant_id", 999)

	err := db.Transaction(func(tx *gorm.DB) error {
		return tx.WithContext(ctxWithKey).Create(strictEntity).Error
	})

	if err != nil {
		t.Fatalf("Expected successful save for strict entity with int key, got error: %v", err)
	}

	var count int64
	db.Model(&TestStrictIntEntity{}).Count(&count)
	if count != 1 {
		t.Errorf("Expected 1 record in DB, got %d", count)
	}

	var auditData map[string]interface{}
	db.Table("test_strict_int_entities_aud").Take(&auditData)
	if auditData["rev_user"] != "999" {
		t.Errorf("Expected rev_user '999', got %v", auditData["rev_user"])
	}
}

func TestAuditing_InvalidTagFormat(t *testing.T) {
	type TestInvalidEntity struct {
		ID      uint   `gorm:"primaryKey"`
		SprinGo string `springo:"audited;invalid_param=123" gorm:"-"`
	}

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	err = db.AutoMigrate(&TestInvalidEntity{})
	if err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	err = EnableAuditing(db, &TestInvalidEntity{})
	if err == nil {
		t.Fatal("Expected EnableAuditing to fail at startup due to unknown parameter, but got nil")
	}

	if !strings.Contains(err.Error(), "unknown parameter") {
		t.Errorf("Expected error to contain 'unknown parameter', got: %v", err)
	}
}

func TestSanitizeRevUser(t *testing.T) {
	longString := strings.Repeat("a", 300)

	testCases := []struct {
		name     string
		input    string
		expected string
	}{
		{name: "Standard valid user", input: "admin_user", expected: "admin_user"},
		{name: "Padded whitespace", input: "  user@domain.com  ", expected: "user@domain.com"},
		{name: "Control characters and newlines", input: "admin\r\nuser\tname", expected: "admin user name"},
		{name: "Null byte stripping", input: "user\x00hacker", expected: "userhacker"},
		{name: "Empty string fallback", input: "", expected: "anonymous"},
		{name: "Whitespace only fallback", input: "    ", expected: "anonymous"},
		{name: "Truncation to 255 chars", input: longString, expected: strings.Repeat("a", 255)},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := sanitizeRevUser(tc.input)
			if result != tc.expected {
				t.Errorf("expected '%s', got '%s'", tc.expected, result)
			}
			if len([]rune(result)) > 255 {
				t.Errorf("result length %d exceeds max 255 chars", len([]rune(result)))
			}
		})
	}
}

func TestAuditDDL_DialectHelpers(t *testing.T) {
	t.Run("MySQL primary key, timestamp, and quoting", func(t *testing.T) {
		if getAuditPKDef("mysql") != "`audit_id` INT AUTO_INCREMENT PRIMARY KEY" {
			t.Errorf("unexpected MySQL PK def: %s", getAuditPKDef("mysql"))
		}
		if getAuditTimestampType("mysql") != "DATETIME" {
			t.Errorf("unexpected MySQL timestamp type: %s", getAuditTimestampType("mysql"))
		}
		if quoteIdentifier("mysql", "users_aud") != "`users_aud`" {
			t.Errorf("unexpected MySQL quoted table: %s", quoteIdentifier("mysql", "users_aud"))
		}
	})

	t.Run("PostgreSQL primary key, timestamp, and quoting", func(t *testing.T) {
		if getAuditPKDef("postgres") != `"audit_id" BIGSERIAL PRIMARY KEY` {
			t.Errorf("unexpected Postgres PK def: %s", getAuditPKDef("postgres"))
		}
		if getAuditTimestampType("postgres") != "TIMESTAMPTZ" {
			t.Errorf("unexpected Postgres timestamp type: %s", getAuditTimestampType("postgres"))
		}
		if quoteIdentifier("postgres", "users_aud") != `"users_aud"` {
			t.Errorf("unexpected Postgres quoted table: %s", quoteIdentifier("postgres", "users_aud"))
		}
	})

	t.Run("SQLite primary key, timestamp, and quoting fallback", func(t *testing.T) {
		if getAuditPKDef("sqlite") != "audit_id INTEGER PRIMARY KEY AUTOINCREMENT" {
			t.Errorf("unexpected SQLite PK def: %s", getAuditPKDef("sqlite"))
		}
		if getAuditTimestampType("sqlite") != "DATETIME" {
			t.Errorf("unexpected SQLite timestamp type: %s", getAuditTimestampType("sqlite"))
		}
		if quoteIdentifier("sqlite", "users_aud") != "users_aud" {
			t.Errorf("unexpected SQLite quoted table: %s", quoteIdentifier("sqlite", "users_aud"))
		}
	})
}
