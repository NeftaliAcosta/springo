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

func TestTransactional_PropagationRequired(t *testing.T) {
	dbFile := "test_tx.db"
	_ = os.Remove(dbFile)

	db, err := gorm.Open(sqlite.Open(dbFile), &gorm.Config{})
	if err != nil {
		t.Fatalf("Failed to open sqlite: %v", err)
	}

	// Clean up physical database file at the end
	defer func() {
		sqlDB, _ := db.DB()
		if sqlDB != nil {
			sqlDB.Close()
		}
		_ = os.Remove(dbFile)
	}()

	// Enable WAL mode & busy timeout on the database connections to allow concurrent write transactions
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

	t.Run("Nested transaction success commits all", func(t *testing.T) {
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
	})

	t.Run("Nested transaction failure rolls back all", func(t *testing.T) {
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
	})

	t.Run("Propagation REQUIRES_NEW commits independently of parent failure", func(t *testing.T) {
		db.Exec("DELETE FROM test_entities")

		err := Transactional(context.Background(), func(ctx1 context.Context) error {
			// Do NOT write in parent transaction before child execution to prevent SQLite lock deadlocks.
			// The child starts and finishes its transaction first.
			errNested := Transactional(ctx1, func(ctx2 context.Context) error {
				return GetTxFromContext(ctx2).Create(&TestEntity{Name: "Inner"}).Error
			}, WithPropagation(PropagationRequiresNew))
			if errNested != nil {
				return errNested
			}

			// Parent writes after child committed
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
	})

	t.Run("Propagation NESTED rolls back only inner on failure", func(t *testing.T) {
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
	})

	t.Run("Propagation SUPPORTS executes with or without tx", func(t *testing.T) {
		db.Exec("DELETE FROM test_entities")

		// Case 1: with parent tx
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

		// Case 2: without parent tx
		err = Transactional(context.Background(), func(ctx1 context.Context) error {
			if GetTxFromContext(ctx1) != nil {
				return errors.New("expected no active tx")
			}
			return nil
		}, WithPropagation(PropagationSupports))
		if err != nil {
			t.Fatalf("Expected no error, got %v", err)
		}
	})

	t.Run("Propagation NOT_SUPPORTED suspends active tx", func(t *testing.T) {
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
	})

	t.Run("Propagation MANDATORY fails if no active tx", func(t *testing.T) {
		err := Transactional(context.Background(), func(ctx context.Context) error {
			return nil
		}, WithPropagation(PropagationMandatory))
		if err == nil {
			t.Fatal("Expected error due to missing active tx, got nil")
		}
	})

	t.Run("Propagation NEVER fails if active tx exists", func(t *testing.T) {
		err := Transactional(context.Background(), func(ctx1 context.Context) error {
			return Transactional(ctx1, func(ctx2 context.Context) error {
				return nil
			}, WithPropagation(PropagationNever))
		})
		if err == nil {
			t.Fatal("Expected error because active tx exists, got nil")
		}
	})

	t.Run("Propagation NESTED event rollback safety", func(t *testing.T) {
		db.Exec("DELETE FROM test_entities")

		var postCommitEvents []interface{}
		RegisterPostCommitHook(func(ctx context.Context, events []interface{}) {
			postCommitEvents = events
		})
		defer RegisterPostCommitHook(nil)

		err := Transactional(context.Background(), func(ctx1 context.Context) error {
			AddEventToTransaction(ctx1, "OuterEvent")

			// Start a nested savepoint transaction and roll it back
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
	})
}
