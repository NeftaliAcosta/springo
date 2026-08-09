package web

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-playground/validator/v10"
)

// StatusCoder interface identifies errors that carry an HTTP status code
type StatusCoder interface {
	error
	StatusCode() int
	ErrorCode() string
	ErrorDetails() any
}

// HandleError centralizes error management (equivalent to @RestControllerAdvice)
func HandleError(w http.ResponseWriter, r *http.Request, err error) {
	ctx := r.Context()
	slog.DebugContext(ctx, "[SprinGo] Handling web request error",
		"error_type", fmt.Sprintf("%T", err),
		"error", err.Error(),
		"path", r.URL.Path,
		"method", r.Method,
	)

	// 1. Validation Errors (422 Unprocessable Entity) — human-readable via translator
	var validationErrors validator.ValidationErrors
	if errors.As(err, &validationErrors) {
		errMap := TranslateValidationErrorsCtx(ctx, validationErrors)
		res := NewErrorResponse(http.StatusUnprocessableEntity, "VALIDATION_ERROR", "Validation Failed", errMap)
		WriteResponse(w, r, http.StatusUnprocessableEntity, res)
		return
	}

	// 2. Polymorphic Business Exceptions (Matches any error with StatusCode() method)
	var sc StatusCoder
	if errors.As(err, &sc) {
		status := sc.StatusCode()
		code := sc.ErrorCode()
		if code == "" {
			code = "BUSINESS_ERROR"
		}

		res := NewErrorResponse(status, code, sc.Error(), sc.ErrorDetails())
		WriteResponse(w, r, status, res)
		return
	}

	// 3. Generic/Unexpected Errors (500 Internal Server Error)
	slog.ErrorContext(ctx, "[SprinGo] Unexpected HTTP server error",
		"error", err.Error(),
		"path", r.URL.Path,
		"method", r.Method,
	)
	res := NewErrorResponse(http.StatusInternalServerError, "INTERNAL_SERVER_ERROR", "An unexpected error occurred", nil)
	WriteResponse(w, r, http.StatusInternalServerError, res)
}
