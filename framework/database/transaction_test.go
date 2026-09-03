package database

import (
	"context"
	"errors"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type TestEntity struct {
	ID   uint `gorm:"primaryKey"`
	Name string
}

// SetupTransactionTestDB initializes a SQLite database connection with WAL mode for transaction testing.
func setupTransactionTestDB(t *testing.T) *gorm.DB {
	dbFile := "test_tx.db"
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

	sqlDB, _ := db.DB()
	if sqlDB != nil {
		sqlDB.SetMaxOpenConns(10)
		db.Exec("PRAGMA journal_mode=WAL;")
		db.Exec("PRAGMA busy_timeout=5000;")
	}

	err = db.AutoMigrate(&TestEntity{})
	if err != nil {
		t.Fatalf("Failed to migrate test entity: %v", err)
	}

	ioc.GetContainer().InitializeAllBeans(db)
	return db
}

func TestTransactional_PropagationRequired(t *testing.T) {
	db := setupTransactionTestDB(t)

	t.Run("Nested transaction success commits all", func(t *testing.T) {
		testTxNestedSuccess(t, db)
	})

	t.Run("Nested transaction failure rolls back all", func(t *testing.T) {
		testTxNestedFailure(t, db)
	})

	t.Run("Propagation REQUIRES_NEW commits independently of parent failure", func(t *testing.T) {
		testTxRequiresNew(t, db)
	})

	t.Run("Propagation NESTED rolls back only inner on failure", func(t *testing.T) {
		testTxNestedSavepointRollback(t, db)
	})

	t.Run("Propagation SUPPORTS executes with or without tx", func(t *testing.T) {
		testTxSupports(t, db)
	})

	t.Run("Propagation NOT_SUPPORTED suspends active tx", func(t *testing.T) {
		testTxNotSupported(t, db)
	})

	t.Run("Propagation MANDATORY fails if no active tx", func(t *testing.T) {
		testTxMandatory(t, db)
	})

	t.Run("Propagation NEVER fails if active tx exists", func(t *testing.T) {
		testTxNever(t, db)
	})

	t.Run("Propagation NESTED event rollback safety", func(t *testing.T) {
		testTxNestedEventRollback(t, db)
	})

	t.Run("Panic in Transactional rolls back physical transaction", func(t *testing.T) {
		testTxPanicRollback(t, db)
	})

	t.Run("GORM error in transaction forces rollback when error returned", func(t *testing.T) {
		testTxGormErrorRollback(t, db)
	})
}

// TestTxNestedSuccess verifies nested REQUIRED transactions commit all entities on success.
func testTxNestedSuccess(t *testing.T, db *gorm.DB) {
	db.Exec("DELETE FROM test_entities")

	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		if err := GetTxFromContext(ctx1).Create(&TestEntity{Name: "Outer"}).Error; err != nil {
			return err
		}

		return Transactional(ctx1, func(ctx2 context.Context) error {
			return GetTxFromContext(ctx2).Create(&TestEntity{Name: "Inner"}).Error
		})
	})

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	var count int64
	db.Model(&TestEntity{}).Count(&count)
	if count != 2 {
		t.Errorf("Expected 2 records in DB, got %d", count)
	}
}

// TestTxNestedFailure verifies nested REQUIRED transaction failure rolls back outer transaction.
func testTxNestedFailure(t *testing.T, db *gorm.DB) {
	db.Exec("DELETE FROM test_entities")

	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		if err := GetTxFromContext(ctx1).Create(&TestEntity{Name: "Outer"}).Error; err != nil {
			return err
		}

		err := Transactional(ctx1, func(ctx2 context.Context) error {
			if err := GetTxFromContext(ctx2).Create(&TestEntity{Name: "Inner"}).Error; err != nil {
				return err
			}
			return errors.New("forced inner failure")
		})
		return err
	})

	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	var count int64
	db.Model(&TestEntity{}).Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 records in DB after rollback, got %d", count)
	}
}

// TestTxRequiresNew verifies REQUIRES_NEW propagation commits independently of parent failure.
func testTxRequiresNew(t *testing.T, db *gorm.DB) {
	db.Exec("DELETE FROM test_entities")

	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		errNested := Transactional(ctx1, func(ctx2 context.Context) error {
			return GetTxFromContext(ctx2).Create(&TestEntity{Name: "Inner"}).Error
		}, WithPropagation(PropagationRequiresNew))
		if errNested != nil {
			return errNested
		}

		if err := GetTxFromContext(ctx1).Create(&TestEntity{Name: "Outer"}).Error; err != nil {
			return err
		}

		return errors.New("parent rollback")
	})

	if err == nil {
		t.Fatal("Expected parent error, got nil")
	}

	var entities []TestEntity
	db.Find(&entities)
	if len(entities) != 1 || entities[0].Name != "Inner" {
		t.Errorf("Expected only 'Inner' to persist, got: %+v", entities)
	}
}

// TestTxNestedSavepointRollback verifies NESTED savepoint rolls back only inner changes.
func testTxNestedSavepointRollback(t *testing.T, db *gorm.DB) {
	db.Exec("DELETE FROM test_entities")

	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		if err := GetTxFromContext(ctx1).Create(&TestEntity{Name: "Outer"}).Error; err != nil {
			return err
		}

		_ = Transactional(ctx1, func(ctx2 context.Context) error {
			GetTxFromContext(ctx2).Create(&TestEntity{Name: "Inner"})
			return errors.New("inner rollback")
		}, WithPropagation(PropagationNested))

		return nil
	})

	if err != nil {
		t.Fatalf("Expected no outer error, got %v", err)
	}

	var entities []TestEntity
	db.Find(&entities)
	if len(entities) != 1 || entities[0].Name != "Outer" {
		t.Errorf("Expected only 'Outer' to persist, got: %+v", entities)
	}
}

// TestTxSupports verifies SUPPORTS propagation executes with or without transaction context.
func testTxSupports(t *testing.T, db *gorm.DB) {
	db.Exec("DELETE FROM test_entities")

	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		return Transactional(ctx1, func(ctx2 context.Context) error {
			if GetTxFromContext(ctx2) == nil {
				return errors.New("expected active tx")
			}
			return nil
		}, WithPropagation(PropagationSupports))
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	err = Transactional(context.Background(), func(ctx1 context.Context) error {
		if GetTxFromContext(ctx1) != nil {
			return errors.New("expected no active tx")
		}
		return nil
	}, WithPropagation(PropagationSupports))
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

// TestTxNotSupported verifies NOT_SUPPORTED propagation suspends active transaction context.
func testTxNotSupported(t *testing.T, db *gorm.DB) {
	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		if GetTxFromContext(ctx1) == nil {
			return errors.New("expected active parent tx")
		}
		return Transactional(ctx1, func(ctx2 context.Context) error {
			if GetTxFromContext(ctx2) != nil {
				return errors.New("expected suspended context (no tx)")
			}
			return nil
		}, WithPropagation(PropagationNotSupported))
	})
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
}

// TestTxMandatory verifies MANDATORY propagation fails when no active transaction exists.
func testTxMandatory(t *testing.T, db *gorm.DB) {
	err := Transactional(context.Background(), func(ctx context.Context) error {
		return nil
	}, WithPropagation(PropagationMandatory))
	if err == nil {
		t.Fatal("Expected error due to missing active tx, got nil")
	}
}

// TestTxNever verifies NEVER propagation fails when an active transaction exists.
func testTxNever(t *testing.T, db *gorm.DB) {
	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		return Transactional(ctx1, func(ctx2 context.Context) error {
			return nil
		}, WithPropagation(PropagationNever))
	})
	if err == nil {
		t.Fatal("Expected error because active tx exists, got nil")
	}
}

// TestTxNestedEventRollback verifies event buffers are truncated upon NESTED savepoint rollback.
func testTxNestedEventRollback(t *testing.T, db *gorm.DB) {
	db.Exec("DELETE FROM test_entities")

	var postCommitEvents []interface{}
	RegisterPostCommitHook(func(ctx context.Context, events []interface{}) {
		postCommitEvents = events
	})
	defer RegisterPostCommitHook(nil)

	err := Transactional(context.Background(), func(ctx1 context.Context) error {
		AddEventToTransaction(ctx1, "OuterEvent")

		nestedErr := Transactional(ctx1, func(ctx2 context.Context) error {
			AddEventToTransaction(ctx2, "NestedEvent")
			return errors.New("nested rollback")
		}, WithPropagation(PropagationNested))

		if nestedErr == nil {
			return errors.New("expected nested error")
		}

		AddEventToTransaction(ctx1, "OuterEventPost")
		return nil
	})

	if err != nil {
		t.Fatalf("Expected outer transaction to commit successfully, got: %v", err)
	}

	assert.Equal(t, []interface{}{"OuterEvent", "OuterEventPost"}, postCommitEvents)
}

// TestTxPanicRollback verifies panics inside Transactional roll back physical changes.
func testTxPanicRollback(t *testing.T, db *gorm.DB) {
	var panicVal interface{}

	func() {
		defer func() {
			panicVal = recover()
		}()

		_ = Transactional(context.Background(), func(ctx context.Context) error {
			tx := GetTxFromContext(ctx)
			if tx == nil {
				t.Fatal("Expected active tx in context")
			}

			tx.Create(&TestEntity{Name: "WillPanic"})
			panic("simulated database panic")
		})
	}()

	if panicVal == nil {
		t.Fatal("Expected panic to re-emerge, got nil")
	}

	var count int64
	db.Model(&TestEntity{}).Where("name = ?", "WillPanic").Count(&count)
	if count != 0 {
		t.Errorf("Expected 0 records after panic rollback, got %d", count)
	}
}

// TestTxGormErrorRollback verifies returning database error forces transaction rollback.
func testTxGormErrorRollback(t *testing.T, db *gorm.DB) {
	db.Exec("DELETE FROM test_entities")

	err := Transactional(context.Background(), func(ctx context.Context) error {
		tx := GetTxFromContext(ctx)
		if tx == nil {
			t.Fatal("Expected active tx in context")
		}

		return tx.Exec("INVALID SQL STATEMENT").Error
	})

	if err == nil {
		t.Fatal("Expected transaction error due to invalid SQL, got nil")
	}
}
