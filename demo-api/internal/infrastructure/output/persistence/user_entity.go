package persistence

import (
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
	"time"

	"gorm.io/gorm"
)

// UserEntity is the persistence model for GORM
type UserEntity struct {
	ID        uint   `gorm:"primarykey"`
	Username  string `gorm:"size:255;uniqueIndex;not null"`
	Email     string `gorm:"size:255;uniqueIndex;not null"`
	CreatedAt time.Time
	UpdatedAt time.Time
	DeletedAt gorm.DeletedAt `gorm:"index"`
	SprinGo   string         `springo:"audited" gorm:"-"`
}

func (UserEntity) TableName() string {
	return "user"
}

// ToDomain converts persistence entity to domain model
func (e *UserEntity) ToDomain() model.User {
	return model.User{
		ID:        e.ID,
		Username:  e.Username,
		Email:     e.Email,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
	}
}

// FromDomain converts domain model to persistence entity
func FromUserDomain(u model.User) UserEntity {
	return UserEntity{
		ID:       u.ID,
		Username: u.Username,
		Email:    u.Email,
	}
}
