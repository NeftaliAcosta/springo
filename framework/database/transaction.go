package database

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/ioc"
	"github.com/NeftaliAcosta/springo/framework/logging"
	"gorm.io/gorm"
)

type contextKey string

const (
	txKey         contextKey = "springo_tx"
	eventsKey     contextKey = "springo_events"
	txReadOnlyKey contextKey = "springo_tx_readonly"
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
	readOnly    bool
}

// TxOption defines configuration overrides for transactional scopes
type TxOption func(*txConfig)

// WithPropagation sets the transaction propagation behavior
func WithPropagation(p Propagation) TxOption {
	return func(cfg *txConfig) {
		cfg.propagation = p
	}
}

// WithReadOnly marks the transaction as read-only.
// When enabled, dialect-specific commands (such as SET TRANSACTION READ ONLY) are issued
// for PostgreSQL and MySQL. In SQLite, the option is safely handled as a no-op.
func WithReadOnly(readOnly ...bool) TxOption {
	return func(cfg *txConfig) {
		if len(readOnly) == 0 {
			cfg.readOnly = true
			return
		}
		cfg.readOnly = readOnly[0]
	}
}

// IsTxReadOnly returns true if the current transaction context is in read-only mode.
func IsTxReadOnly(ctx context.Context) bool {
	if ro, ok := ctx.Value(txReadOnlyKey).(bool); ok {
		return ro
	}
	return false
}

// GetTx extracts the active transaction from context or returns fallback DB if provided.
func GetTx(ctx context.Context, fallback ...*gorm.DB) *gorm.DB {
	if tx := GetTxFromContext(ctx); tx != nil {
		return tx
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return nil
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
// It matches Spring Boot's propagation model and supports read-only transaction mode.
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
			return executeInActiveTx(ctx, activeTx, fn)
		}
		return executeInNewTx(ctx, fn, cfg.readOnly)

	case PropagationRequiresNew:
		return executeInRequiresNew(ctx, activeTx, fn, cfg.readOnly)

	case PropagationNested:
		if activeTx != nil {
			return executeInNestedTx(ctx, activeTx, fn)
		}
		return executeInNewTx(ctx, fn, cfg.readOnly)

	case PropagationSupports:
		return fn(ctx)

	case PropagationNotSupported:
		return executeInNotSupported(ctx, activeTx, fn)

	case PropagationMandatory:
		if activeTx == nil {
			return fmt.Errorf("transaction propagation MANDATORY failed: no active transaction found in context")
		}
		return executeInActiveTx(ctx, activeTx, fn)

	case PropagationNever:
		if activeTx != nil {
			return fmt.Errorf("transaction propagation NEVER failed: active transaction found in context")
		}
		return fn(ctx)

	default:
		return fmt.Errorf("unknown transaction propagation mode: %d", cfg.propagation)
	}
}

// ExecuteInActiveTx runs fn within an existing active transaction with error and panic recovery.
func executeInActiveTx(ctx context.Context, activeTx *gorm.DB, fn func(ctx context.Context) error) error {
	defer func() {
		if r := recover(); r != nil {
			_ = activeTx.AddError(fmt.Errorf("transaction panic: %v", r))
			panic(r)
		}
	}()

	if err := fn(ctx); err != nil {
		_ = activeTx.AddError(err)
		return err
	}

	return nil
}

// ExecuteInRequiresNew suspends active transaction context and executes fn in a new physical transaction.
func executeInRequiresNew(
	ctx context.Context,
	activeTx *gorm.DB,
	fn func(ctx context.Context) error,
	readOnly bool,
) error {
	suspendedCtx := ctx
	if activeTx != nil {
		suspendedCtx = context.WithValue(ctx, txKey, nil)
		suspendedCtx = context.WithValue(suspendedCtx, eventsKey, nil)
		suspendedCtx = context.WithValue(suspendedCtx, txReadOnlyKey, false)
	}
	return executeInNewTx(suspendedCtx, fn, readOnly)
}

// ExecuteInNotSupported suspends active transaction context and executes fn without transaction.
func executeInNotSupported(ctx context.Context, activeTx *gorm.DB, fn func(ctx context.Context) error) error {
	if activeTx != nil {
		suspendedCtx := context.WithValue(ctx, txKey, nil)
		suspendedCtx = context.WithValue(suspendedCtx, eventsKey, nil)
		suspendedCtx = context.WithValue(suspendedCtx, txReadOnlyKey, false)
		return fn(suspendedCtx)
	}
	return fn(ctx)
}

// ExecuteInNestedTx executes fn within a nested transaction savepoint with rollback guarantees.
func executeInNestedTx(ctx context.Context, activeTx *gorm.DB, fn func(ctx context.Context) error) error {
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
				slog.Warn(fmt.Sprintf("⚠️ [Transaction] Rollback to savepoint %s failed on panic", spName),
					slog.String(logging.SubsystemKey, logging.FrameworkSubsystem),
					slog.String("savepoint", spName),
					slog.Any("error", err))
			}
			if ok && buffer != nil {
				*buffer = (*buffer)[:initialLen]
			}
			panic(r)
		}
	}()

	if executionErr := fn(ctx); executionErr != nil {
		if rollbackErr := activeTx.RollbackTo(spName).Error; rollbackErr != nil {
			return fmt.Errorf(
				"transaction execution error: %v, and failed to rollback to savepoint %s: %w",
				executionErr,
				spName,
				rollbackErr,
			)
		}
		if ok && buffer != nil {
			*buffer = (*buffer)[:initialLen]
		}
		return executionErr
	}

	return nil
}

// GetReadOnlyTxSQL returns the dialect-specific SQL statement to configure a read-only transaction.
func getReadOnlyTxSQL(dialector string) string {
	switch strings.ToLower(dialector) {
	case "postgres", "postgresql", "mysql":
		return "SET TRANSACTION READ ONLY"
	default:
		return ""
	}
}

// ApplyReadOnlyMode executes dialect-specific directives to configure transaction as read-only.
func applyReadOnlyMode(tx *gorm.DB) error {
	sqlStatement := getReadOnlyTxSQL(tx.Name())
	if sqlStatement == "" {
		return nil
	}
	return tx.Exec(sqlStatement).Error
}

// ExecuteInNewTx starts a new physical GORM transaction and executes fn with rollback safety on error or panic.
func executeInNewTx(ctx context.Context, fn func(ctx context.Context) error, readOnly bool) error {
	db := ioc.GetContainer().GetDB()
	if db == nil {
		return fmt.Errorf("transaction failed: primary database connection not found in container")
	}

	var tx *gorm.DB
	if readOnly {
		tx = db.WithContext(ctx).Begin(&sql.TxOptions{ReadOnly: true})
	} else {
		tx = db.WithContext(ctx).Begin()
	}
	if tx.Error != nil {
		return fmt.Errorf("failed to start transaction: %w", tx.Error)
	}

	if readOnly {
		if err := applyReadOnlyMode(tx); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("failed to set read-only transaction: %w", err)
		}
	}

	defer func() {
		if r := recover(); r != nil {
			_ = tx.Rollback()
			panic(r)
		}
	}()

	var eventBuffer []interface{}
	txCtx := context.WithValue(ctx, txKey, tx)
	txCtx = context.WithValue(txCtx, eventsKey, &eventBuffer)
	txCtx = context.WithValue(txCtx, txReadOnlyKey, readOnly)

	if err := fn(txCtx); err != nil {
		_ = tx.Rollback()
		return err
	}

	if tx.Error != nil {
		_ = tx.Rollback()
		return fmt.Errorf("transaction aborted due to GORM error: %w", tx.Error)
	}

	if err := tx.Commit().Error; err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("failed to commit transaction: %w", err)
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
