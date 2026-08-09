package data

import (
	"context"
	"github.com/NeftaliAcosta/springo/framework/database"

	"gorm.io/gorm"
)

// BaseRepository provides common CRUD operations using Go Generics.
type BaseRepository[T any] struct {
	db *gorm.DB
}

// NewBaseRepository creates a new generic repository instance.
func NewBaseRepository[T any](db *gorm.DB) BaseRepository[T] {
	return BaseRepository[T]{db: db}
}

// GetDB returns the transaction if present in context, otherwise the standard connection.
func (r *BaseRepository[T]) GetDB(ctx context.Context) *gorm.DB {
	if tx := database.GetTxFromContext(ctx); tx != nil {
		return tx
	}
	return r.db
}

// Save inserts or updates an entity with context support.
func (r *BaseRepository[T]) Save(ctx context.Context, entity *T) error {
	return r.GetDB(ctx).Save(entity).Error
}

// FindAll retrieves all records of type T with context support.
func (r *BaseRepository[T]) FindAll(ctx context.Context) ([]T, error) {
	var results []T
	err := r.GetDB(ctx).Find(&results).Error
	return results, err
}

// FindByID retrieves a single record by its ID with context support.
func (r *BaseRepository[T]) FindByID(ctx context.Context, id uint) (*T, error) {
	var result T
	err := r.GetDB(ctx).First(&result, id).Error
	if err != nil {
		return nil, err
	}
	return &result, nil
}

// Delete removes a record by its ID with context support.
func (r *BaseRepository[T]) Delete(ctx context.Context, id uint) error {
	var entity T
	return r.GetDB(ctx).Delete(&entity, id).Error
}
