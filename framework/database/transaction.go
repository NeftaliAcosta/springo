package database

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/ioc"
	"log"

	"gorm.io/gorm"
)

type contextKey string

const (
	txKey     contextKey = "springo_tx"
	eventsKey contextKey = "springo_events"
)

// Propagation defines transaction propagation behaviors matching Spring Boot
type Propagation int

const (
	// PropagationRequired (Default) joins existing tx or starts new one
	PropagationRequired Propagation = iota
	// PropagationRequiresNew always starts a new physical tx, suspending active one
	PropagationRequiresNew
	// PropagationNested runs within nested savepoint if active tx exists
	PropagationNested
	// PropagationSupports runs within active tx if exists, else without tx
	PropagationSupports
	// PropagationNotSupported runs without tx, suspending active tx if exists
	PropagationNotSupported
	// PropagationMandatory requires active tx, errors if none exists
	PropagationMandatory
	// PropagationNever forbids active tx, errors if one exists
	PropagationNever
)

type txConfig struct {
	propagation Propagation
}

// TxOption defines configuration overrides for transactional scopes
type TxOption func(*txConfig)

// WithPropagation sets the transaction propagation behavior
func WithPropagation(p Propagation) TxOption {
	return func(cfg *txConfig) {
		cfg.propagation = p
	}
}

// PostCommitHook is a function that runs after a transaction commit
type PostCommitHook func(ctx context.Context, events []interface{})

// TransactionalEvent wraps an event with its physical outbox ID if applicable
type TransactionalEvent struct {
	Event    interface{}
	OutboxID uint
}

var onPostCommit PostCommitHook

// RegisterPostCommitHook sets the function to call after a successful commit
func RegisterPostCommitHook(hook PostCommitHook) {
	onPostCommit = hook
}

// Transactional wraps a function in a database transaction with configurable propagation.
// It matches Spring Boot's propagation model.
func Transactional(ctx context.Context, fn func(ctx context.Context) error, opts ...TxOption) error {
	cfg := &txConfig{
		propagation: PropagationRequired,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	activeTx := GetTxFromContext(ctx)

	switch cfg.propagation {
	case PropagationRequired:
		if activeTx != nil {
			return fn(ctx)
		}
		return executeInNewTx(ctx, fn)

	case PropagationRequiresNew:
		suspendedCtx := ctx
		if activeTx != nil {
			// Suspend active transaction by removing transaction context variables
			suspendedCtx = context.WithValue(ctx, txKey, nil)
			suspendedCtx = context.WithValue(suspendedCtx, eventsKey, nil)
		}
		return executeInNewTx(suspendedCtx, fn)

	case PropagationNested:
		if activeTx != nil {
			spName := fmt.Sprintf("sp_%p_%d", &fn, activeTx.RowsAffected)
			if err := activeTx.SavePoint(spName).Error; err != nil {
				return fmt.Errorf("failed to establish savepoint %s: %w", spName, err)
			}
			var initialLen int
			buffer, ok := ctx.Value(eventsKey).(*[]interface{})
			if ok && buffer != nil {
				initialLen = len(*buffer)
			}

			defer func() {
				if r := recover(); r != nil {
					if err := activeTx.RollbackTo(spName).Error; err != nil {
						log.Printf("⚠️ [Transaction] Rollback to savepoint %s failed on panic: %v", spName, err)
					}
					if ok && buffer != nil {
						*buffer = (*buffer)[:initialLen]
					}
					panic(r)
				}
			}()

			if executionErr := fn(ctx); executionErr != nil {
				if rollbackErr := activeTx.RollbackTo(spName).Error; rollbackErr != nil {
					return fmt.Errorf("transaction execution error: %v, and failed to rollback to savepoint %s: %w", executionErr, spName, rollbackErr)
				}
				if ok && buffer != nil {
					*buffer = (*buffer)[:initialLen]
				}
				return executionErr
			}
			return nil
		}
		return executeInNewTx(ctx, fn)

	case PropagationSupports:
		return fn(ctx)

	case PropagationNotSupported:
		if activeTx != nil {
			// Suspend active transaction for this block execution
			suspendedCtx := context.WithValue(ctx, txKey, nil)
			suspendedCtx = context.WithValue(suspendedCtx, eventsKey, nil)
			return fn(suspendedCtx)
		}
		return fn(ctx)

	case PropagationMandatory:
		if activeTx == nil {
			return fmt.Errorf("transaction propagation MANDATORY failed: no active transaction found in context")
		}
		return fn(ctx)

	case PropagationNever:
		if activeTx != nil {
			return fmt.Errorf("transaction propagation NEVER failed: active transaction found in context")
		}
		return fn(ctx)

	default:
		return fmt.Errorf("unknown transaction propagation mode: %d", cfg.propagation)
	}
}

func executeInNewTx(ctx context.Context, fn func(ctx context.Context) error) error {
	db := ioc.GetContainer().GetDB()
	if db == nil {
		return fmt.Errorf("transaction failed: primary database connection not found in container")
	}

	tx := db.Begin()
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	var eventBuffer []interface{}
	txCtx := context.WithValue(ctx, txKey, tx)
	txCtx = context.WithValue(txCtx, eventsKey, &eventBuffer)

	if err := fn(txCtx); err != nil {
		tx.Rollback()
		return err
	}

	if err := tx.Commit().Error; err != nil {
		return err
	}

	if onPostCommit != nil {
		onPostCommit(ctx, eventBuffer)
	}

	return nil
}

// AddEventToTransaction adds an event to the current transaction's buffer
func AddEventToTransaction(ctx context.Context, event interface{}) bool {
	buffer, ok := ctx.Value(eventsKey).(*[]interface{})
	if ok {
		*buffer = append(*buffer, event)
		return true
	}
	return false
}

// GetTxFromContext extracts the GORM transaction from context if it exists
func GetTxFromContext(ctx context.Context) *gorm.DB {
	if tx, ok := ctx.Value(txKey).(*gorm.DB); ok {
		return tx
	}
	return nil
}

// RunInTx executes a function inside a transaction block and returns both the result and error.
// It uses Go Generics to eliminate boilerplate variable declarations outside closures.
func RunInTx[T any](ctx context.Context, fn func(ctx context.Context) (T, error), opts ...TxOption) (T, error) {
	var result T
	err := Transactional(ctx, func(txCtx context.Context) error {
		var err error
		result, err = fn(txCtx)
		return err
	}, opts...)
	return result, err
}
