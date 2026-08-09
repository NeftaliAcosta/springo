package mappers

import (
	"github.com/NeftaliAcosta/springo/demo-api/internal/domain/model"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/request"
	"github.com/NeftaliAcosta/springo/demo-api/internal/infrastructure/dtos/response"
)

// ToUserDomain converts a UserRequestDTO into a User domain model
func ToUserDomain(dto request.UserRequestDTO) model.User {
	return model.User{
		Username: dto.Username,
		Email:    dto.Email,
	}
}

// UpdateUserDomain updates a domain user with data from Update DTO
func UpdateUserDomain(user *model.User, dto request.UserUpdateRequestDTO) {
	user.Username = dto.Username
	user.Email = dto.Email
}

// ToUserResponseDTO maps a domain User to a UserResponseDTO
func ToUserResponseDTO(user model.User) response.UserResponseDTO {
	return response.UserResponseDTO{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
	}
}

// ToUserResponseDTOList maps a slice of domain Users to a slice of DTOs
func ToUserResponseDTOList(users []model.User) []response.UserResponseDTO {
	dtosList := make([]response.UserResponseDTO, len(users))
	for i, user := range users {
		dtosList[i] = ToUserResponseDTO(user)
	}
	return dtosList
}
