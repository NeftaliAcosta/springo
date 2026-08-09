package in

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
)

// UserUseCase is an inbound port (Interface for business logic)
type UserUseCase interface {
	CreateUser(ctx context.Context, dto request.UserRequestDTO) (response.UserResponseDTO, error)
	GetAllUsers(ctx context.Context) ([]response.UserResponseDTO, error)
	GetUserByID(ctx context.Context, id uint) (response.UserResponseDTO, error)
	UpdateUser(ctx context.Context, dto request.UserUpdateRequestDTO) (response.UserResponseDTO, error)
	TriggerBusinessError(ctx context.Context) error
	ComplexRegistration(ctx context.Context, dto request.UserRequestDTO) error
}
