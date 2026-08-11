package web

import (
	"context"
	"fmt"
	"github.com/NeftaliAcosta/springo/framework/database"
	"github.com/NeftaliAcosta/springo/framework/errors"
	"github.com/NeftaliAcosta/springo/framework/security"
	"mime"
	"net/http"
	"reflect"
)

// ─── Dispatch Options ────────────────────────────────────────────────────────

// dispatchConfig holds options for a Dispatch call.
type dispatchConfig struct {
	roles            []string
	validationGroups []interface{}
}

// DispatchOption is a functional option for configuring Dispatch behavior.
type DispatchOption func(*dispatchConfig)

// WithRoles restricts the endpoint to users with at least one of the given roles.
// Equivalent to Spring Boot's @PreAuthorize("hasAnyRole('ADMIN', 'MANAGER')").
func WithRoles(roles ...string) DispatchOption {
	return func(c *dispatchConfig) {
		c.roles = roles
	}
}

// WithValidationGroup sets the validation groups to apply when validating request DTOs.
// Equivalent to Spring Boot's @Validated(OnCreate.class).
//
// Example:
//
//	web.Dispatch(c.create, web.WithValidationGroup(web.OnCreate{}))
//	web.Dispatch(c.update, web.WithValidationGroup(web.OnUpdate{}))
func WithValidationGroup(groups ...interface{}) DispatchOption {
	return func(c *dispatchConfig) {
		c.validationGroups = groups
	}
}

// ─── applyOptions ─────────────────────────────────────────────────────────────

func applyOptions(rawArgs []interface{}) *dispatchConfig {
	cfg := &dispatchConfig{}
	for _, arg := range rawArgs {
		switch v := arg.(type) {
		case DispatchOption:
			v(cfg)
		case string:
			// Backward compatibility: bare string args treated as roles
			cfg.roles = append(cfg.roles, v)
		}
	}
	return cfg
}

// ─── Dispatch ─────────────────────────────────────────────────────────────────

// Dispatch handles dynamic parameter resolution (using HandlerMethodArgumentResolver), DTO validation,
// security role checks, and error/success response formatting.
//
// Supports legacy role strings and new DispatchOption functional options:
//
//	// Legacy (still works):
//	web.Dispatch(c.create, "ADMIN")
//
//	// New style with options:
//	web.Dispatch(c.create, web.WithRoles("ADMIN"), web.WithValidationGroup(web.OnCreate{}))
func Dispatch(fn any, opts ...any) http.HandlerFunc {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()
	if fnType.Kind() != reflect.Func {
		panic("web.Dispatch: fn must be a function")
	}

	if fnType.NumOut() < 1 || fnType.NumOut() > 2 {
		panic("web.Dispatch: fn must return (value, error) or (error)")
	}

	cfg := applyOptions(opts)

	return func(w http.ResponseWriter, r *http.Request) {
		defer cleanupMultipartRequest(r)
		if !checkAuthorization(w, r, cfg.roles) {
			return
		}

		args, ok := resolveArguments(w, r, fnType, cfg.validationGroups)
		if !ok {
			return
		}

		executeAndRespond(w, r, fnVal, args)
	}
}

// ─── Authorization ────────────────────────────────────────────────────────────

func checkAuthorization(w http.ResponseWriter, r *http.Request, requiredRoles []string) bool {
	if len(requiredRoles) == 0 {
		return true
	}

	val := r.Context().Value(security.RolesContextKey)
	if val == nil {
		HandleError(w, r, errors.Forbidden("access denied: no roles found", "AUTH_NO_ROLES"))
		return false
	}

	userRoles, ok := val.([]string)
	if !ok {
		HandleError(w, r, errors.Forbidden("access denied: invalid roles format", "AUTH_INVALID_FORMAT"))
		return false
	}

	if !hasRequiredRole(requiredRoles, userRoles) {
		HandleError(w, r, errors.Forbidden("insufficient permissions", "AUTH_INSUFFICIENT_PERMISSIONS"))
		return false
	}

	return true
}

func hasRequiredRole(required []string, userRoles []string) bool {
	for _, req := range required {
		if containsRole(userRoles, req) {
			return true
		}
	}
	return false
}

func containsRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}

// ─── Argument Resolution ──────────────────────────────────────────────────────

func resolveArguments(w http.ResponseWriter, r *http.Request, fnType reflect.Type, groups []interface{}) ([]reflect.Value, bool) {
	args := make([]reflect.Value, fnType.NumIn())

	for i := 0; i < fnType.NumIn(); i++ {
		paramType := fnType.In(i)

		// 1. Special case: context.Context
		if paramType.Implements(reflect.TypeOf((*context.Context)(nil)).Elem()) {
			args[i] = reflect.ValueOf(r.Context())
			continue
		}

		// 2. Try registered resolvers
		val, resolved, err := resolveSingleArgument(paramType, r)
		if err != nil {
			HandleError(w, r, err)
			return nil, false
		}
		if resolved {
			if !val.IsValid() {
				args[i] = reflect.Zero(paramType)
			} else {
				args[i] = val
			}
			continue
		}

		// 3. Fallback: bind + validate as DTO (with optional group)
		dtoVal, ok := bindAndValidateDTO(w, r, paramType, groups)
		if !ok {
			return nil, false
		}
		args[i] = dtoVal
	}

	return args, true
}

func resolveSingleArgument(paramType reflect.Type, r *http.Request) (reflect.Value, bool, error) {
	for _, resolver := range getArgumentResolvers() {
		if resolver.SupportsParameter(paramType) {
			val, err := resolver.ResolveArgument(paramType, r)
			if err != nil {
				return reflect.Value{}, false, err
			}
			if val == nil {
				return reflect.Value{}, true, nil
			}
			return reflect.ValueOf(val), true, nil
		}
	}
	return reflect.Value{}, false, nil
}

func bindAndValidateDTO(w http.ResponseWriter, r *http.Request, paramType reflect.Type, groups []interface{}) (reflect.Value, bool) {
	isPtr := paramType.Kind() == reflect.Ptr
	baseType := paramType
	if isPtr {
		baseType = paramType.Elem()
	}

	// Backward compatibility: support empty placeholder interface{} or any
	if baseType.Kind() == reflect.Interface {
		return reflect.Zero(paramType), true
	}

	if baseType.Kind() != reflect.Struct {
		err := errors.InternalServer(fmt.Sprintf("unsupported controller parameter type: %v", paramType), "WEB_UNSUPPORTED_PARAMETER")
		HandleError(w, r, err)
		return reflect.Value{}, false
	}

	structVal := reflect.New(baseType)

	// Decode the body according to its media type. Requests without an explicit
	// Content-Type preserve the historical JSON fallback.
	if r.Method != http.MethodGet && r.ContentLength != 0 {
		mediaType, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if mediaType == "multipart/form-data" {
			if err := BindMultipartRequest(w, r, structVal.Interface()); err != nil {
				HandleError(w, r, err)
				return reflect.Value{}, false
			}
		} else {
			if !DecodeJSON(w, r, structVal.Interface()) {
				return reflect.Value{}, false
			}
		}
	}

	// Bind Path and Query parameters
	if err := BindRequest(r, structVal.Interface()); err != nil {
		HandleError(w, r, err)
		return reflect.Value{}, false
	}

	// Execute Validation with optional groups
	if err := Validate(structVal.Interface(), groups...); err != nil {
		HandleError(w, r, err)
		return reflect.Value{}, false
	}

	if isPtr {
		return structVal, true
	}
	return structVal.Elem(), true
}

func cleanupMultipartRequest(r *http.Request) {
	if r.MultipartForm != nil {
		_ = r.MultipartForm.RemoveAll()
	}
}

// ─── Execute & Respond ────────────────────────────────────────────────────────

func executeAndRespond(w http.ResponseWriter, r *http.Request, fnVal reflect.Value, args []reflect.Value) {
	results := fnVal.Call(args)

	var errVal reflect.Value
	var resVal reflect.Value

	if len(results) == 1 {
		errVal = results[0]
	} else {
		resVal = results[0]
		errVal = results[1]
	}

	if !errVal.IsNil() {
		err := errVal.Interface().(error)
		HandleError(w, r, err)
		return
	}

	var responseData any
	if resVal.IsValid() {
		responseData = resVal.Interface()
	}

	status := calculateSuccessStatus(r.Method)
	WriteResponse(w, r, status, NewSuccessResponse(status, responseData))
}

func calculateSuccessStatus(method string) int {
	if method == http.MethodPost {
		return http.StatusCreated
	}
	return http.StatusOK
}

// ─── DispatchTx ───────────────────────────────────────────────────────────────

// DispatchTx operates exactly like Dispatch, but automatically wraps the controller and service call
// inside a database transaction, rolling back automatically if an error occurs.
// This is Go's equivalent to Spring Boot's @Transactional annotation on a Controller/Endpoint.
//
// Supports DispatchOption functional options:
//
//	web.DispatchTx(c.create, web.WithRoles("ADMIN"), web.WithValidationGroup(web.OnCreate{}))
func DispatchTx(fn any, opts ...any) http.HandlerFunc {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()
	if fnType.Kind() != reflect.Func {
		panic("web.DispatchTx: fn must be a function")
	}

	if fnType.NumOut() < 1 || fnType.NumOut() > 2 {
		panic("web.DispatchTx: fn must return (value, error) or (error)")
	}

	cfg := applyOptions(opts)

	return func(w http.ResponseWriter, r *http.Request) {
		if !checkAuthorization(w, r, cfg.roles) {
			return
		}

		err := database.Transactional(r.Context(), func(txCtx context.Context) error {
			r = r.WithContext(txCtx)

			args, ok := resolveArguments(w, r, fnType, cfg.validationGroups)
			if !ok {
				return fmt.Errorf("argument resolution failed")
			}

			results := fnVal.Call(args)
			var errVal reflect.Value
			var resVal reflect.Value

			if len(results) == 1 {
				errVal = results[0]
			} else {
				resVal = results[0]
				errVal = results[1]
			}

			if !errVal.IsNil() {
				return errVal.Interface().(error)
			}

			var responseData any
			if resVal.IsValid() {
				responseData = resVal.Interface()
			}
			status := calculateSuccessStatus(r.Method)
			WriteResponse(w, r, status, NewSuccessResponse(status, responseData))
			return nil
		})

		if err != nil {
			HandleError(w, r, err)
		}
	}
}
