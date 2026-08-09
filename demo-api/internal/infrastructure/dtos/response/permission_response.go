package response

// PermissionResponseDTO represents a list of user permissions
type PermissionResponseDTO struct {
	UserID      string   `json:"user_id"`
	Permissions []string `json:"permissions"`
	Source      string   `json:"source"` // To show if it came from Cache or DB
}
