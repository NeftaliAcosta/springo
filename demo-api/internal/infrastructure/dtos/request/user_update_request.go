package request

// UserUpdateRequestDTO is used for updating an existing user (PUT).
// Fields are validated against the OnUpdate group.
// Password is omitted — not updatable via this endpoint.
type UserUpdateRequestDTO struct {
	ID       uint   `path:"id"       validate:"required,gt=0"`
	Username string `json:"username" validate:"required,min=3,max=50"`
	Email    string `json:"email"    validate:"required,email"`
}
