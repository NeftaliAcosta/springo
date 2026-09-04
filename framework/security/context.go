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

// GetRoles returns the granted authorities / roles from context.
func GetRoles(ctx context.Context) []string {
	if ctx == nil {
		return nil
	}
	if roles, ok := ctx.Value(rolesCtxKey).([]string); ok {
		return roles
	}
	// Fallback to legacy context key if present
	if roles, ok := ctx.Value(RolesContextKey).([]string); ok {
		return roles
	}
	return nil
}

// GetClaims returns all JWT claims stored in context.
func GetClaims(ctx context.Context) map[string]any {
	if ctx == nil {
		return nil
	}
	if claims, ok := ctx.Value(claimsCtxKey).(map[string]any); ok {
		return claims
	}
	// Fallback to legacy context key if present
	if claims, ok := ctx.Value(ClaimsContextKey).(map[string]any); ok {
		return claims
	}
	return nil
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
