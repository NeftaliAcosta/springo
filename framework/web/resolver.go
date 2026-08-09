package web

import (
	"github.com/NeftaliAcosta/springo/framework/security"
	"net/http"
	"reflect"
	"sync"
)

// TraceID is a custom type to allow dynamic injection of the Trace ID parameter.
type TraceID string

// HandlerMethodArgumentResolver defines the contract for resolving controller method arguments dynamically.
type HandlerMethodArgumentResolver interface {
	// SupportsParameter checks if the resolver can handle the given parameter type.
	SupportsParameter(paramType reflect.Type) bool
	// ResolveArgument extracts and returns the argument value from the request.
	ResolveArgument(paramType reflect.Type, r *http.Request) (any, error)
}

type resolverRegistry struct {
	mu        sync.RWMutex
	resolvers []HandlerMethodArgumentResolver
}

var resolversInstance = &resolverRegistry{}

// RegisterArgumentResolver adds a new custom argument resolver to the framework.
func RegisterArgumentResolver(resolver HandlerMethodArgumentResolver) {
	resolversInstance.mu.Lock()
	defer resolversInstance.mu.Unlock()
	resolversInstance.resolvers = append(resolversInstance.resolvers, resolver)
}

// getArgumentResolvers returns all registered resolvers.
func getArgumentResolvers() []HandlerMethodArgumentResolver {
	resolversInstance.mu.RLock()
	defer resolversInstance.mu.RUnlock()
	// Return a copy to avoid concurrent slice modifications issues
	copied := make([]HandlerMethodArgumentResolver, len(resolversInstance.resolvers))
	copy(copied, resolversInstance.resolvers)
	return copied
}

// ClearArgumentResolvers clears all registered resolvers and re-registers the defaults.
func ClearArgumentResolvers() {
	resolversInstance.mu.Lock()
	defer resolversInstance.mu.Unlock()
	resolversInstance.resolvers = nil
	resolversInstance.resolvers = append(resolversInstance.resolvers,
		&UserInfoResolver{},
		&HeaderResolver{},
		&RequestResolver{},
		&TraceIDResolver{},
	)
}

func init() {
	registerDefaultResolvers()
}

func registerDefaultResolvers() {
	RegisterArgumentResolver(&UserInfoResolver{})
	RegisterArgumentResolver(&HeaderResolver{})
	RegisterArgumentResolver(&RequestResolver{})
	RegisterArgumentResolver(&TraceIDResolver{})
}

// UserInfoResolver automatically resolves *security.UserInfo parameters from the authenticated JWT context.
type UserInfoResolver struct{}

func (u *UserInfoResolver) SupportsParameter(paramType reflect.Type) bool {
	return paramType == reflect.TypeOf((*security.UserInfo)(nil))
}

func (u *UserInfoResolver) ResolveArgument(paramType reflect.Type, r *http.Request) (any, error) {
	ctx := r.Context()
	username, _ := ctx.Value(security.UserContextKey).(string)
	roles, _ := ctx.Value(security.RolesContextKey).([]string)
	claims, _ := ctx.Value(security.ClaimsContextKey).(map[string]any)

	// If the JWT token was not validated or no user context exists, return nil
	if username == "" && len(roles) == 0 && claims == nil {
		return (*security.UserInfo)(nil), nil
	}

	return &security.UserInfo{
		Username: username,
		Roles:    roles,
		Claims:   claims,
	}, nil
}

// HeaderResolver automatically injects the HTTP request headers (http.Header).
type HeaderResolver struct{}

func (h *HeaderResolver) SupportsParameter(paramType reflect.Type) bool {
	return paramType == reflect.TypeOf((*http.Header)(nil)).Elem()
}

func (h *HeaderResolver) ResolveArgument(paramType reflect.Type, r *http.Request) (any, error) {
	return r.Header, nil
}

// RequestResolver automatically injects the raw HTTP request (*http.Request).
type RequestResolver struct{}

func (r *RequestResolver) SupportsParameter(paramType reflect.Type) bool {
	return paramType == reflect.TypeOf((*http.Request)(nil))
}

func (r *RequestResolver) ResolveArgument(paramType reflect.Type, req *http.Request) (any, error) {
	return req, nil
}

// TraceIDResolver automatically injects the request TraceID (web.TraceID).
type TraceIDResolver struct{}

func (t *TraceIDResolver) SupportsParameter(paramType reflect.Type) bool {
	return paramType == reflect.TypeOf((*TraceID)(nil)).Elem()
}

func (t *TraceIDResolver) ResolveArgument(paramType reflect.Type, r *http.Request) (any, error) {
	return TraceID(GetTraceID(r.Context())), nil
}
