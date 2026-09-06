package security_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/NeftaliAcosta/springo/framework/security"
	"github.com/golang-jwt/jwt/v5"
)

func TestOAuth2PropertiesDevProfile(t *testing.T) {
	_ = os.Setenv("SPRINGO_PROFILES_ACTIVE", "dev")
	defer func() { _ = os.Unsetenv("SPRINGO_PROFILES_ACTIVE") }()

	props := &security.OAuth2ResourceServerProperties{
		Algorithm: "RS256",
	}
	if err := props.Validate(); err != nil {
		t.Fatalf("expected no validation error in dev, got %v", err)
	}
}

func TestOAuth2PropertiesProdProfileRS256(t *testing.T) {
	_ = os.Setenv("SPRINGO_PROFILES_ACTIVE", "prod")
	defer func() { _ = os.Unsetenv("SPRINGO_PROFILES_ACTIVE") }()

	props := &security.OAuth2ResourceServerProperties{
		Algorithm: "RS256",
	}
	if err := props.Validate(); err == nil {
		t.Fatalf("expected validation error in prod when no jwks/issuer/public-key configured")
	}

	props.IssuerURI = "https://issuer.example.com/realm"
	if err := props.Validate(); err != nil {
		t.Fatalf("expected valid configuration with IssuerURI, got %v", err)
	}
}

func TestOAuth2PropertiesProdProfileHS256(t *testing.T) {
	_ = os.Setenv("SPRINGO_PROFILES_ACTIVE", "prod")
	defer func() { _ = os.Unsetenv("SPRINGO_PROFILES_ACTIVE") }()

	props := &security.OAuth2ResourceServerProperties{
		Algorithm: "HS256",
		Secret:    "short",
	}
	if err := props.Validate(); err == nil {
		t.Fatalf("expected error for short secret in prod")
	}

	props.Secret = "this-is-a-very-long-secret-key-that-exceeds-32-bytes"
	if err := props.Validate(); err != nil {
		t.Fatalf("expected valid HS256 secret, got %v", err)
	}
}

func TestOAuth2PropertiesDisabled(t *testing.T) {
	_ = os.Setenv("SPRINGO_PROFILES_ACTIVE", "prod")
	defer func() { _ = os.Unsetenv("SPRINGO_PROFILES_ACTIVE") }()

	disabled := false
	props := &security.OAuth2ResourceServerProperties{
		Enabled:   &disabled,
		Algorithm: "RS256",
	}
	if err := props.Validate(); err != nil {
		t.Fatalf("expected no error when disabled, got %v", err)
	}
}

func TestPrincipalExtractionPreferredUsername(t *testing.T) {
	claims := jwt.MapClaims{
		"preferred_username": "john_doe",
		"sub":                "user-uuid-1234",
	}
	principal := security.ExtractPrincipal(claims, "preferred_username")
	if principal != "john_doe" {
		t.Fatalf("expected 'john_doe', got %q", principal)
	}
}

func TestPrincipalExtractionFallbackToSub(t *testing.T) {
	claims := jwt.MapClaims{
		"sub": "user-uuid-1234",
	}
	principal := security.ExtractPrincipal(claims, "preferred_username")
	if principal != "user-uuid-1234" {
		t.Fatalf("expected 'user-uuid-1234', got %q", principal)
	}
}

func TestRolesExtractionDirectAuthoritiesClaim(t *testing.T) {
	claims := jwt.MapClaims{
		"scope_roles": []interface{}{"ADMIN", "ROLE_USER"},
	}
	roles := security.ExtractRoles(claims, "scope_roles", "", "ROLE_")
	if len(roles) != 2 || roles[0] != "ROLE_ADMIN" || roles[1] != "ROLE_USER" {
		t.Fatalf("unexpected roles: %v", roles)
	}
}

func TestRolesExtractionKeycloakWithResourceID(t *testing.T) {
	claims := jwt.MapClaims{
		"resource_access": map[string]interface{}{
			"my-service": map[string]interface{}{
				"roles": []interface{}{"BILLING_MANAGER"},
			},
		},
	}
	roles := security.ExtractRoles(claims, "", "my-service", "ROLE_")
	if len(roles) != 1 || roles[0] != "ROLE_BILLING_MANAGER" {
		t.Fatalf("unexpected roles: %v", roles)
	}
}

func TestRolesExtractionKeycloakFallbackToAzp(t *testing.T) {
	claims := jwt.MapClaims{
		"azp": "frontend-client",
		"resource_access": map[string]interface{}{
			"frontend-client": map[string]interface{}{
				"roles": []string{"VIEWER"},
			},
		},
	}
	roles := security.ExtractRoles(claims, "", "", "ROLE_")
	if len(roles) != 1 || roles[0] != "ROLE_VIEWER" {
		t.Fatalf("unexpected roles: %v", roles)
	}
}

func TestIssuerValidator(t *testing.T) {
	v := &security.IssuerValidator{ExpectedIssuer: "https://auth.example.com/realm"}

	err := v.Validate(context.Background(), jwt.MapClaims{"iss": "https://auth.example.com/realm/"})
	if err != nil {
		t.Fatalf("expected valid matching issuer, got %v", err)
	}

	err = v.Validate(context.Background(), jwt.MapClaims{"iss": "https://attacker.com"})
	if err == nil {
		t.Fatalf("expected error for mismatching issuer")
	}
}

func TestAudienceValidatorMatchString(t *testing.T) {
	v := &security.AudienceValidator{AllowedAudiences: []string{"my-api-service"}}
	if err := v.Validate(context.Background(), jwt.MapClaims{"aud": "my-api-service"}); err != nil {
		t.Fatalf("expected valid aud string, got %v", err)
	}
}

func TestAudienceValidatorMatchSlice(t *testing.T) {
	v := &security.AudienceValidator{AllowedAudiences: []string{"my-api-service"}}
	claimsSlice := jwt.MapClaims{"aud": []interface{}{"other", "my-api-service"}}
	if err := v.Validate(context.Background(), claimsSlice); err != nil {
		t.Fatalf("expected valid aud slice, got %v", err)
	}
}

func TestAudienceValidatorMatchAzp(t *testing.T) {
	v := &security.AudienceValidator{
		AllowedAudiences:         []string{"my-api-service"},
		AllowedAuthorizedParties: []string{"client-app"},
	}
	if err := v.Validate(context.Background(), jwt.MapClaims{"aud": "my-api-service", "azp": "client-app"}); err != nil {
		t.Fatalf("expected valid aud and azp, got %v", err)
	}

	// Invalid when aud does not match even if azp matches
	if err := v.Validate(context.Background(), jwt.MapClaims{"aud": "other-api", "azp": "client-app"}); err == nil {
		t.Fatalf("expected error when audience does not match")
	}
}

func TestAudienceValidatorMismatch(t *testing.T) {
	v := &security.AudienceValidator{AllowedAudiences: []string{"my-api-service"}}
	if err := v.Validate(context.Background(), jwt.MapClaims{"aud": "wrong-client"}); err == nil {
		t.Fatalf("expected error for mismatching audience")
	}
}

func TestCustomTokenValidatorRegistry(t *testing.T) {
	security.ClearTokenValidators()
	defer security.ClearTokenValidators()

	custom := &mockValidator{shouldFail: true}
	security.RegisterTokenValidator(custom)

	vals := security.GetTokenValidators()
	if len(vals) != 1 {
		t.Fatalf("expected 1 validator registered, got %d", len(vals))
	}

	err := vals[0].Validate(context.Background(), jwt.MapClaims{})
	if err == nil {
		t.Fatalf("expected custom validator failure")
	}
}

func TestSecurityContext(t *testing.T) {
	ctx := context.Background()
	user := "alice"
	roles := []string{"ROLE_USER", "ROLE_ADMIN"}
	claims := map[string]any{"sub": "alice", "custom": "value123"}
	token := "raw-jwt-token-string"

	secCtx := security.WithSecurityContext(ctx, user, roles, claims, token)

	if got := security.GetUser(secCtx); got != user {
		t.Fatalf("expected user %q, got %q", user, got)
	}
	if got := security.GetRoles(secCtx); len(got) != 2 || got[0] != "ROLE_USER" {
		t.Fatalf("unexpected roles: %v", got)
	}
	if got := security.GetBearerToken(secCtx); got != token {
		t.Fatalf("expected token %q, got %q", token, got)
	}
	if got := security.GetClaims(secCtx); got["custom"] != "value123" {
		t.Fatalf("unexpected claims: %v", got)
	}
}

func TestSecurityContextBlankFallback(t *testing.T) {
	if got := security.GetUser(context.Background()); got != "" {
		t.Fatalf("expected empty user for blank context, got %q", got)
	}
}

type mockValidator struct {
	shouldFail bool
}

func (m *mockValidator) Validate(_ context.Context, _ jwt.MapClaims) error {
	if m.shouldFail {
		return fmt.Errorf("custom validation failure")
	}
	return nil
}

func TestGenerateAndValidateTokenWithOAuth2(t *testing.T) {
	secret := "test-secret-key-with-sufficient-length-for-hmac-sha256"
	provider := security.NewJwtProvider(secret, 15)

	tokenStr, err := provider.GenerateTokenWithClaims("test_user", []string{"ADMIN"}, map[string]interface{}{
		"preferred_username": "test_username",
	})
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	token, err := provider.ValidateToken(tokenStr)
	if err != nil || !token.Valid {
		t.Fatalf("failed to validate token: %v", err)
	}

	claims := token.Claims.(jwt.MapClaims)
	principal := security.ExtractPrincipal(claims, "preferred_username")
	if principal != "test_username" {
		t.Fatalf("expected principal 'test_username', got %q", principal)
	}
}

func TestTokenExpiration(t *testing.T) {
	secret := "test-secret-key-with-sufficient-length-for-hmac-sha256"
	provider := security.NewJwtProvider(secret, -1) // Already expired.

	tokenStr, err := provider.GenerateToken("expired_user", []string{"USER"})
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	_, err = provider.ValidateToken(tokenStr)
	if err == nil {
		t.Fatalf("expected validation error for expired token")
	}
}
