package persistence

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/application/port/output"
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
	"github.com/NeftaliAcosta/springo/framework/data"
	"github.com/NeftaliAcosta/springo/framework/ioc"

	"gorm.io/gorm"
)

type userRepositoryAdapter struct {
	data.BaseRepository[UserEntity]
}

// init registers the adapter to the IoC container
func init() {
	ioc.GetContainer().RegisterRepositoryFactory("UserRepository", func(dbConn *gorm.DB) interface{} {
		return &userRepositoryAdapter{
			BaseRepository: data.NewBaseRepository[UserEntity](dbConn),
		}
	})
}

func (a *userRepositoryAdapter) Save(ctx context.Context, user *model.User) error {
	entity := FromUserDomain(*user)
	err := a.BaseRepository.Save(ctx, &entity)
	user.ID = entity.ID // Update domain ID after save
	return err
}

func (a *userRepositoryAdapter) FindAll(ctx context.Context) ([]model.User, error) {
	entities, err := a.BaseRepository.FindAll(ctx)
	if err != nil {
		return nil, err
	}

	users := make([]model.User, len(entities))
	for i, e := range entities {
		users[i] = e.ToDomain()
	}
	return users, nil
}

func (a *userRepositoryAdapter) FindByID(ctx context.Context, id uint) (*model.User, error) {
	entity, err := a.BaseRepository.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	u := entity.ToDomain()
	return &u, nil
}

func (a *userRepositoryAdapter) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var entity UserEntity
	err := a.BaseRepository.GetDB(ctx).Where("username = ?", username).First(&entity).Error
	if err != nil {
		return nil, err
	}
	u := entity.ToDomain()
	return &u, nil
}

func (a *userRepositoryAdapter) Delete(ctx context.Context, id uint) error {
	return a.BaseRepository.Delete(ctx, id)
}

func (a *userRepositoryAdapter) ExistsByEmail(ctx context.Context, email string) (bool, error) {
	var count int64
	err := a.BaseRepository.GetDB(ctx).
		Model(&UserEntity{}).
		Where("email = ?", email).
		Count(&count).Error
	return count > 0, err
}

// Ensure interface implementation
var _ output.UserPersistencePort = (*userRepositoryAdapter)(nil)
