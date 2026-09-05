package security

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecurityContext_NilContext(t *testing.T) {
	var nilCtx context.Context

	assert.Empty(t, GetUser(nilCtx))
	assert.Nil(t, GetRoles(nilCtx))
	assert.False(t, HasRole(nilCtx, "ROLE_ADMIN"))
	assert.False(t, HasAnyRole(nilCtx, "ROLE_ADMIN", "ROLE_USER"))
	assert.Nil(t, GetClaims(nilCtx))
	assert.Empty(t, GetBearerToken(nilCtx))
	assert.Nil(t, GetUserInfo(nilCtx))

	val, ok := GetClaim[string](nilCtx, "email")
	assert.False(t, ok)
	assert.Empty(t, val)
}

func TestSecurityContext_WithSecurityContextAndGetters(t *testing.T) {
	claims := map[string]any{
		"sub":       "user-123",
		"email":     "user@example.com",
		"tenant_id": "tenant-xyz",
		"login_cnt": 5,
	}
	roles := []string{"ROLE_USER", "ROLE_ADMIN"}
	token := "header.payload.signature"

	ctx := WithSecurityContext(context.Background(), "user-123", roles, claims, token)

	assert.Equal(t, "user-123", GetUser(ctx))
	assert.Equal(t, roles, GetRoles(ctx))
	assert.Equal(t, token, GetBearerToken(ctx))

	assert.True(t, HasRole(ctx, "ROLE_ADMIN"))
	assert.True(t, HasRole(ctx, "ROLE_USER"))
	assert.False(t, HasRole(ctx, "ROLE_SUPERUSER"))

	assert.True(t, HasAnyRole(ctx, "ROLE_GUEST", "ROLE_ADMIN"))
	assert.False(t, HasAnyRole(ctx, "ROLE_GUEST", "ROLE_MODERATOR"))

	email, ok := GetClaim[string](ctx, "email")
	assert.True(t, ok)
	assert.Equal(t, "user@example.com", email)

	count, ok := GetClaim[int](ctx, "login_cnt")
	assert.True(t, ok)
	assert.Equal(t, 5, count)

	missing, ok := GetClaim[string](ctx, "non_existent")
	assert.False(t, ok)
	assert.Empty(t, missing)

	wrongType, ok := GetClaim[int](ctx, "email")
	assert.False(t, ok)
	assert.Zero(t, wrongType)

	info := GetUserInfo(ctx)
	assert.NotNil(t, info)
	assert.Equal(t, "user-123", info.Username)
	assert.Equal(t, roles, info.Roles)
	assert.Equal(t, "user@example.com", info.Claims["email"])
}

func TestSecurityContext_Immutability(t *testing.T) {
	claims := map[string]any{
		"email": "user@example.com",
	}
	roles := []string{"ROLE_USER"}

	ctx := WithSecurityContext(context.Background(), "user-1", roles, claims, "token-1")

	// 1. Mutate returned claims map
	retrievedClaims := GetClaims(ctx)
	retrievedClaims["email"] = "hacked@example.com"
	retrievedClaims["new_key"] = "hacked_value"

	freshClaims := GetClaims(ctx)
	assert.Equal(t, "user@example.com", freshClaims["email"])
	assert.Nil(t, freshClaims["new_key"])

	// 2. Mutate returned roles slice
	retrievedRoles := GetRoles(ctx)
	retrievedRoles[0] = "ROLE_SUPERADMIN"

	freshRoles := GetRoles(ctx)
	assert.Equal(t, "ROLE_USER", freshRoles[0])
}

func TestSecurityContext_LegacyContextKeyFallback(t *testing.T) {
	legacyCtx := context.WithValue(context.Background(), UserContextKey, "legacy-user")
	legacyCtx = context.WithValue(legacyCtx, RolesContextKey, []string{"ROLE_LEGACY"})
	legacyCtx = context.WithValue(legacyCtx, ClaimsContextKey, map[string]any{"legacy_claim": "legacy_val"})

	assert.Equal(t, "legacy-user", GetUser(legacyCtx))
	assert.Equal(t, []string{"ROLE_LEGACY"}, GetRoles(legacyCtx))
	assert.True(t, HasRole(legacyCtx, "ROLE_LEGACY"))

	val, ok := GetClaim[string](legacyCtx, "legacy_claim")
	assert.True(t, ok)
	assert.Equal(t, "legacy_val", val)

	claims := GetClaims(legacyCtx)
	assert.Equal(t, "legacy_val", claims["legacy_claim"])
}

func TestSecurityContext_BearerTokenDownstreamPattern(t *testing.T) {
	token := "eyJh...sample-token"
	ctx := WithSecurityContext(context.Background(), "user-service", []string{"ROLE_SERVICE"}, nil, token)

	extractedBearer := GetBearerToken(ctx)
	authHeaderValue := "Bearer " + extractedBearer
	assert.Equal(t, "Bearer eyJh...sample-token", authHeaderValue)
}

func TestSecurityContext_ConcurrentReads(t *testing.T) {
	claims := map[string]any{
		"sub":   "user-concurrent",
		"roles": []string{"ROLE_A", "ROLE_B"},
	}
	roles := []string{"ROLE_A", "ROLE_B"}
	ctx := WithSecurityContext(context.Background(), "user-concurrent", roles, claims, "jwt-token")

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			assert.Equal(t, "user-concurrent", GetUser(ctx))
			assert.True(t, HasRole(ctx, "ROLE_A"))
			assert.Equal(t, "jwt-token", GetBearerToken(ctx))

			c := GetClaims(ctx)
			c["mutated"] = true // safe because GetClaims returns clone

			r := GetRoles(ctx)
			r[0] = "MUTATED" // safe because GetRoles returns slice copy
		}()
	}
	wg.Wait()
}
