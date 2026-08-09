package request

// UserDetailRequestDTO for fetching a user by ID
type UserDetailRequestDTO struct {
	ID uint `path:"id" validate:"required,gt=0"`
}
