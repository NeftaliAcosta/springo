package response

// UserResponseDTO for sending user data back to the client
type UserResponseDTO struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}
