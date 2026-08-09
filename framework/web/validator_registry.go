package web

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
)

// ConstraintValidator is the contract for custom field validators.
// T is the field type being validated (string, int, etc.).
// Equivalent to Spring Boot's ConstraintValidator<Annotation, T>.
type ConstraintValidator[T any] interface {
	// IsValid returns (valid bool, errorMessage string).
	// ctx carries the HTTP request context (JWT claims, trace ID, etc.).
	IsValid(value T, ctx context.Context) (bool, string)
}

var (
	customValidatorsMu sync.RWMutex
	registeredTags     = map[string]struct{}{} // track registered tags to avoid duplicates
)

// RegisterValidator registers a custom validator function under a validate tag name.
// The tag can then be used in struct tags: validate:"unique_email"
//
// Example:
//
//	web.RegisterValidator("unique_email", &UniqueEmailValidator{UserRepo: repo})
//
// Must be called AFTER framework.Bootstrap() so the IoC container is initialized.
func RegisterValidator[T any](tag string, v ConstraintValidator[T]) error {
	customValidatorsMu.Lock()
	defer customValidatorsMu.Unlock()

	if _, exists := registeredTags[tag]; exists {
		return fmt.Errorf("validator tag '%s' already registered", tag)
	}

	var zero T
	targetType := reflect.TypeOf(&zero).Elem()

	err := validate.RegisterValidationCtx(tag, func(ctx context.Context, fl validator.FieldLevel) bool {
		fieldVal := fl.Field()

		if !fieldVal.Type().AssignableTo(targetType) && !fieldVal.Type().ConvertibleTo(targetType) {
			return false
		}

		typedVal := fieldVal.Convert(targetType).Interface().(T)
		valid, _ := v.IsValid(typedVal, ctx)
		return valid
	})
	if err != nil {
		return fmt.Errorf("failed to register validator '%s': %w", tag, err)
	}

	// Register human-readable translation using the concrete ut.Translator type
	registerCustomTranslation(tag, v)

	registeredTags[tag] = struct{}{}
	return nil
}

// registerCustomTranslation registers a translation for a custom validator tag.
// Uses the correct concrete function signatures expected by go-playground/validator.
func registerCustomTranslation[T any](tag string, v ConstraintValidator[T]) {
	_ = validate.RegisterTranslation(tag, trans,
		func(ut ut.Translator) error {
			return ut.Add(tag, "{0} "+tag+" validation failed", true)
		},
		func(ut ut.Translator, fe validator.FieldError) string {
			// Probe the validator with a zero value to get the custom message
			var zero T
			_, msg := v.IsValid(zero, context.Background())
			if msg != "" {
				t, _ := ut.T(tag, fe.Field())
				_ = t
				return fmt.Sprintf("%s: %s", fe.Field(), msg)
			}
			t, _ := ut.T(tag, fe.Field())
			return t
		},
	)
}
