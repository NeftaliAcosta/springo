package service

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/application/port/output"
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
	"testing"
)

// MockUserRepository is a manual mock for unit testing
type MockUserRepository struct {
	output.UserPersistencePort // Embed to satisfy interface, implement only what we need
	SaveFunc                   func(ctx context.Context, user *model.User) error
	FindByUsernameFunc         func(ctx context.Context, username string) (*model.User, error)
}

func (m *MockUserRepository) Save(ctx context.Context, user *model.User) error {
	if m.SaveFunc != nil {
		return m.SaveFunc(ctx, user)
	}
	return nil
}

func (m *MockUserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	if m.FindByUsernameFunc != nil {
		return m.FindByUsernameFunc(ctx, username)
	}
	return nil, nil
}

func TestUserService_CreateUser_Success(t *testing.T) {
	// 1. Arrange: Create the Mock Repository
	mockRepo := &MockUserRepository{
		FindByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			// Simulate user does not exist
			return nil, nil
		},
		SaveFunc: func(ctx context.Context, user *model.User) error {
			// Simulate successful save
			return nil
		},
	}

	// Create the service manually (NO framework, NO database)
	service := NewUserService(mockRepo)
	ctx := context.Background()
	dto := request.UserRequestDTO{Username: "testuser", Email: "test@go.com"}

	// 2. Act
	response, err := service.CreateUser(ctx, dto)

	// 3. Assert
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if response.Username != "testuser" {
		t.Errorf("Expected username 'testuser', got '%s'", response.Username)
	}
}

func TestUserService_CreateUser_AlreadyExists(t *testing.T) {
	// 1. Arrange
	mockRepo := &MockUserRepository{
		FindByUsernameFunc: func(ctx context.Context, username string) (*model.User, error) {
			// Simulate user ALREADY exists
			return &model.User{}, nil
		},
	}

	service := NewUserService(mockRepo)
	ctx := context.Background()
	dto := request.UserRequestDTO{Username: "existinguser"}

	// 2. Act
	_, err := service.CreateUser(ctx, dto)

	// 3. Assert
	if err == nil {
		t.Errorf("Expected an error (UserAlreadyExists), got nil")
	}
}
