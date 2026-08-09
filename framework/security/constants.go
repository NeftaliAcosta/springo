package security

type ContextKey string

const (
	UserContextKey   ContextKey = "user"
	RolesContextKey  ContextKey = "roles"
	ClaimsContextKey ContextKey = "claims"
)

// UserInfo represents the authenticated user details extracted from JWT.
type UserInfo struct {
	Username string
	Roles    []string
	Claims   map[string]any
}
