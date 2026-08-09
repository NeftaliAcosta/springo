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

func TestAuditing_CreateUpdateDelete(t *testing.T) {
	// 1. Setup in-memory SQLite DB
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	// 2. Auto-migrate base tables
	err = db.AutoMigrate(&TestAuditedEntity{}, &TestNonAuditedEntity{}, &TestStrictEntity{}, &TestStrictIntEntity{})
	if err != nil {
		t.Fatalf("Failed to auto-migrate: %v", err)
	}

	// 3. Enable Auditing on the connection
	err = EnableAuditing(db, &TestAuditedEntity{}, &TestNonAuditedEntity{}, &TestStrictEntity{}, &TestStrictIntEntity{})
	if err != nil {
		t.Fatalf("Failed to enable auditing: %v", err)
	}

	// Verify that test_audited_entities_aud exists and test_non_audited_entities_aud does not
	if !db.Migrator().HasTable("test_audited_entities_aud") {
		t.Error("Expected table test_audited_entities_aud to be created")
	}
	if db.Migrator().HasTable("test_non_audited_entities_aud") {
		t.Error("Did not expect table test_non_audited_entities_aud to be created")
	}

	// 4. Test CREATE (INSERT)
	ctx := context.WithValue(context.Background(), "username", "admin_user")
	entity := &TestAuditedEntity{Name: "Juan", Email: "juan@example.com"}

	err = db.WithContext(ctx).Create(entity).Error
	if err != nil {
		t.Fatalf("Failed to create record: %v", err)
	}

	// Verify audit record for INSERT
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

	// 5. Test UPDATE
	entity.Name = "Juan Carlos"
	err = db.WithContext(ctx).Save(entity).Error
	if err != nil {
		t.Fatalf("Failed to update record: %v", err)
	}

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

	// 6. Test DELETE (with anonymous fallback)
	err = db.Delete(entity).Error
	if err != nil {
		t.Fatalf("Failed to delete record: %v", err)
	}

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

	// 7. Verify non-audited table does not produce audit records
	nonAudited := &TestNonAuditedEntity{Name: "SkipMe"}
	err = db.Create(nonAudited).Error
	if err != nil {
		t.Fatalf("Failed to create non-audited record: %v", err)
	}

	// Still should have no audit table and no errors
	if db.Migrator().HasTable("test_non_audited_entities_aud") {
		t.Error("Non-audited entity should not have an audit table")
	}

	// 8. Verify STRICT user_key mode (abort on missing key)
	t.Run("Strict mode aborts on missing key", func(t *testing.T) {
		strictEntity := &TestStrictEntity{Name: "Strict No Key"}
		ctxEmpty := context.Background()

		// Should fail due to missing context key "company_email"
		err = db.Transaction(func(tx *gorm.DB) error {
			return tx.WithContext(ctxEmpty).Create(strictEntity).Error
		})

		if err == nil {
			t.Error("Expected error when saving strict entity without required context key, but got nil")
		}

		// Verify record was not inserted (rollback worked)
		var count int64
		db.Model(&TestStrictEntity{}).Count(&count)
		if count != 0 {
			t.Errorf("Expected 0 records in DB after strict rollback, got %d", count)
		}
	})

	t.Run("Strict mode succeeds on correct key", func(t *testing.T) {
		strictEntity := &TestStrictEntity{Name: "Strict With Key"}
		ctxWithKey := context.WithValue(context.Background(), "company_email", "admin@acme.com")

		err = db.Transaction(func(tx *gorm.DB) error {
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
	})

	t.Run("Strict mode succeeds on non-string (int/float64) key", func(t *testing.T) {
		strictEntity := &TestStrictIntEntity{Name: "Strict With Int Key"}
		ctxWithKey := context.WithValue(context.Background(), "tenant_id", 999) // int type

		err = db.Transaction(func(tx *gorm.DB) error {
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
	})
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
