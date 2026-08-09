package response

// LoginResponseDTO represents the response returned after successful authentication
type LoginResponseDTO struct {
	Token string   `json:"token"`
	Roles []string `json:"roles"`
}
