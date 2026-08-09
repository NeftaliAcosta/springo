package service

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/demo-api/internal/application/port/output"
	domainErrors "github.com/NeftaliAcosta/springo/demo-api/internal/domain/errors"
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/port/in"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/mappers"
	"github.com/NeftaliAcosta/springo/framework/database"
	"github.com/NeftaliAcosta/springo/framework/errors"
	"github.com/NeftaliAcosta/springo/framework/ioc"
)

type userServiceImpl struct {
	repo output.UserPersistencePort
}

// init registers the service factory to the IoC container
func init() {
	ioc.GetContainer().RegisterServiceFactory("UserService", func() interface{} {
		repo := ioc.GetContainer().GetBean("UserRepository").(output.UserPersistencePort)
		return NewUserService(repo)
	})
}

// NewUserService creates a new instance of the application service
func NewUserService(repo output.UserPersistencePort) in.UserUseCase {
	return &userServiceImpl{repo: repo}
}

func (s *userServiceImpl) CreateUser(ctx context.Context, dto request.UserRequestDTO) (response.UserResponseDTO, error) {
	// 1. Check if user already exists
	existing, _ := s.repo.FindByUsername(ctx, dto.Username)
	if existing != nil {
		return response.UserResponseDTO{}, domainErrors.UserAlreadyExists(dto.Username)
	}

	user := mappers.ToUserDomain(dto)
	err := s.repo.Save(ctx, &user)
	return mappers.ToUserResponseDTO(user), err
}

func (s *userServiceImpl) GetAllUsers(ctx context.Context) ([]response.UserResponseDTO, error) {
	users, err := s.repo.FindAll(ctx)
	if err != nil {
		return nil, err
	}
	return mappers.ToUserResponseDTOList(users), nil
}

func (s *userServiceImpl) GetUserByID(ctx context.Context, id uint) (response.UserResponseDTO, error) {
	user, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return response.UserResponseDTO{}, errors.NotFound("User not found", "USER_NOT_FOUND")
	}
	return mappers.ToUserResponseDTO(*user), nil
}

func (s *userServiceImpl) UpdateUser(ctx context.Context, dto request.UserUpdateRequestDTO) (response.UserResponseDTO, error) {
	user, err := s.repo.FindByID(ctx, dto.ID)
	if err != nil {
		return response.UserResponseDTO{}, errors.NotFound("User not found for update", "USER_NOT_FOUND")
	}

	mappers.UpdateUserDomain(user, dto)
	err = s.repo.Save(ctx, user)
	return mappers.ToUserResponseDTO(*user), err
}

func (s *userServiceImpl) TriggerBusinessError(ctx context.Context) error {
	// Example of a transactional method with Rollback
	return database.Transactional(ctx, func(txCtx context.Context) error {
		// 1. Save something
		user := mappers.ToUserDomain(request.UserRequestDTO{
			Username: "transactional_test_user",
			Email:    "test@example.com",
		})
		s.repo.Save(txCtx, &user)

		// 2. Force error to trigger Rollback
		return fmt.Errorf("forced error to trigger @Transactional rollback")
	})
}

func (s *userServiceImpl) ComplexRegistration(ctx context.Context, dto request.UserRequestDTO) error {
	// 1. Outer transaction starts (REQUIRED)
	return database.Transactional(ctx, func(txCtx context.Context) error {
		// 1.1 Save a user attempt log inside a nested REQUIRES_NEW transaction.
		// It commits independently, so if registration fails, the attempt remains logged.
		logErr := database.Transactional(txCtx, func(logCtx context.Context) error {
			logUser := mappers.ToUserDomain(request.UserRequestDTO{
				Username: "LOG-" + dto.Username,
				Email:    dto.Email,
			})
			return s.repo.Save(logCtx, &logUser)
		}, database.WithPropagation(database.PropagationRequiresNew))

		if logErr != nil {
			return fmt.Errorf("failed to write audit log: %w", logErr)
		}

		// 1.2 Save the actual user
		user := mappers.ToUserDomain(dto)
		if err := s.repo.Save(txCtx, &user); err != nil {
			return err
		}

		// 1.3 Simulate error to trigger rollback of outer user entity (LOG-user remains saved)
		if dto.Username == "rollback-me" {
			return fmt.Errorf("forced business error for demo rollback")
		}

		return nil
	})
}

var _ in.UserUseCase = (*userServiceImpl)(nil)
