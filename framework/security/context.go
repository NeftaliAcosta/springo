package security

import (
	"context"
)

// Private context key type to prevent collision across packages.
type securityContextKey struct {
	name string
}

var (
	userCtxKey        = securityContextKey{name: "user"}
	rolesCtxKey       = securityContextKey{name: "roles"}
	claimsCtxKey      = securityContextKey{name: "claims"}
	bearerTokenCtxKey = securityContextKey{name: "bearerToken"}
)

// GetUser returns the authenticated principal name from context.
func GetUser(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if user, ok := ctx.Value(userCtxKey).(string); ok {
		return user
	}
	// Fallback to legacy context key if present
	if user, ok := ctx.Value(UserContextKey).(string); ok {
		return user
	}
	return ""
}

// GetRoles returns a defensive copy of granted authorities / roles from context.
func GetRoles(ctx context.Context) []string {
	raw := extractRawRoles(ctx)
	if len(raw) == 0 {
		return nil
	}
	roles := make([]string, len(raw))
	copy(roles, raw)
	return roles
}

// HasRole verifies whether the authenticated context contains the specified role.
func HasRole(ctx context.Context, role string) bool {
	for _, r := range extractRawRoles(ctx) {
		if r == role {
			return true
		}
	}
	return false
}

// HasAnyRole verifies whether the authenticated context contains any of the specified roles.
func HasAnyRole(ctx context.Context, roles ...string) bool {
	targetRoles := extractRawRoles(ctx)
	for _, target := range targetRoles {
		for _, r := range roles {
			if target == r {
				return true
			}
		}
	}
	return false
}

// GetClaims returns a defensive copy of all JWT claims stored in context.
func GetClaims(ctx context.Context) map[string]any {
	raw := extractRawClaims(ctx)
	if raw == nil {
		return nil
	}
	copied := make(map[string]any, len(raw))
	for k, v := range raw {
		copied[k] = v
	}
	return copied
}

// GetClaim retrieves a specific typed claim value from context without cloning the full map.
func GetClaim[T any](ctx context.Context, key string) (T, bool) {
	var zero T
	raw := extractRawClaims(ctx)
	if raw == nil {
		return zero, false
	}
	val, exists := raw[key]
	if !exists {
		return zero, false
	}
	typed, ok := val.(T)
	if !ok {
		return zero, false
	}
	return typed, true
}

// GetBearerToken returns the raw bearer token from context for downstream propagation.
func GetBearerToken(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if token, ok := ctx.Value(bearerTokenCtxKey).(string); ok {
		return token
	}
	return ""
}

// GetUserInfo returns consolidated authenticated principal details from context.
func GetUserInfo(ctx context.Context) *UserInfo {
	user := GetUser(ctx)
	if user == "" {
		return nil
	}
	return &UserInfo{
		Username: user,
		Roles:    GetRoles(ctx),
		Claims:   GetClaims(ctx),
	}
}

// WithSecurityContext attaches authenticated user details to the given context.
func WithSecurityContext(
	ctx context.Context,
	user string,
	roles []string,
	claims map[string]any,
	bearerToken string,
) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}

	ctx = context.WithValue(ctx, userCtxKey, user)
	ctx = context.WithValue(ctx, rolesCtxKey, roles)
	ctx = context.WithValue(ctx, claimsCtxKey, claims)
	ctx = context.WithValue(ctx, bearerTokenCtxKey, bearerToken)

	// Keep legacy keys populated for backward compatibility
	ctx = context.WithValue(ctx, UserContextKey, user)
	ctx = context.WithValue(ctx, RolesContextKey, roles)
	ctx = context.WithValue(ctx, ClaimsContextKey, claims)

	return ctx
}

func extractRawRoles(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	if roles, ok := ctx.Value(rolesCtxKey).([]string); ok {
		return roles
	}
	if roles, ok := ctx.Value(RolesContextKey).([]string); ok {
		return roles
	}
	return nil
}

func extractRawClaims(ctx context.Context) map[string]any {
	if ctx == nil {
		return nil
	}
	if claims, ok := ctx.Value(claimsCtxKey).(map[string]any); ok {
		return claims
	}
	if claims, ok := ctx.Value(ClaimsContextKey).(map[string]any); ok {
		return claims
	}
	return nil
}
