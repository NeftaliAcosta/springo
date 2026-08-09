package request

// LoginRequestDTO represents credentials for authentication
type LoginRequestDTO struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}
