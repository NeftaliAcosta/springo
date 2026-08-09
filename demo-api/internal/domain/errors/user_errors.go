package errors

import f_errors "github.com/NeftaliAcosta/springo/framework/errors"

// UserAlreadyExistsError is a custom domain exception
type UserAlreadyExistsError struct {
	*f_errors.ConflictError
}

// UserAlreadyExists standardizes the creation of the error without the "New" prefix
func UserAlreadyExists(username string) error {
	return &UserAlreadyExistsError{
		ConflictError: f_errors.Conflict("User with username '"+username+"' already exists", "USER_ALREADY_EXISTS"),
	}
}
