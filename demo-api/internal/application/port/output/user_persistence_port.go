package output

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
)

// UserPersistencePort is an outbound port (Interface for persistence)
type UserPersistencePort interface {
	Save(ctx context.Context, user *model.User) error
	FindAll(ctx context.Context) ([]model.User, error)
	FindByID(ctx context.Context, id uint) (*model.User, error)
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	ExistsByEmail(ctx context.Context, email string) (bool, error)
	Delete(ctx context.Context, id uint) error
}
