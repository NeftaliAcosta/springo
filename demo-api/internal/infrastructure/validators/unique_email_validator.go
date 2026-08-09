package validators

import (
	"context"
	"github.com/NeftaliAcosta/springo/demo-api/internal/application/port/output"
)

// UniqueEmailValidator checks that an email address is not already registered.
// Implements web.ConstraintValidator[string].
// Registered under the validate tag: unique_email
//
// Usage in DTO struct tags:
//
//	Email string `json:"email" validate:"required,email,unique_email"`
type UniqueEmailValidator struct {
	UserRepo output.UserPersistencePort
}

// IsValid queries the database to check email uniqueness.
// Returns (false, "email already registered") if email exists.
func (v *UniqueEmailValidator) IsValid(email string, ctx context.Context) (bool, string) {
	if email == "" {
		return true, "" // Let the 'required' tag handle empty case
	}

	exists, err := v.UserRepo.ExistsByEmail(ctx, email)
	if err != nil {
		// On DB error, fail open (let request through) and log internally
		return true, ""
	}
	if exists {
		return false, "email already registered"
	}
	return true, ""
}
