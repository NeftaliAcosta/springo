package request

import "github.com/NeftaliAcosta/springo/framework/web"

// UserRequestDTO is used for creating a new user (POST).
// Fields are validated against the OnCreate group.
type UserRequestDTO struct {
	// required on create, must be unique
	Username string `json:"username" validate:"required,min=3,max=50"`
	// required on create, must pass unique_email custom validator
	Email string `json:"email"    validate:"required,email,unique_email"`
	// required on create only
	Password string `json:"password" validate:"required,min=8"`
}

// Ensure groups are accessible without import cycle
var _ web.ValidationGroup = web.OnCreate{}
