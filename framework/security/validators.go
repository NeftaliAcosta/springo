package security

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/golang-jwt/jwt/v5"
)

// TokenValidator validates custom claims after cryptographic signature verification.
type TokenValidator interface {
	Validate(ctx context.Context, claims jwt.MapClaims) error
}

var (
	validatorsMu sync.RWMutex
	validators   []TokenValidator
)

// RegisterTokenValidator registers a custom token validator into the execution chain.
func RegisterTokenValidator(v TokenValidator) {
	if v == nil {
		return
	}
	validatorsMu.Lock()
	defer validatorsMu.Unlock()
	validators = append(validators, v)
}

// ClearTokenValidators clears all custom validators (primarily for testing).
func ClearTokenValidators() {
	validatorsMu.Lock()
	defer validatorsMu.Unlock()
	validators = nil
}

// GetTokenValidators returns a snapshot copy of registered validators.
func GetTokenValidators() []TokenValidator {
	validatorsMu.RLock()
	defer validatorsMu.RUnlock()
	list := make([]TokenValidator, len(validators))
	copy(list, validators)
	return list
}

// IssuerValidator validates that the token 'iss' claim matches the expected issuer URI.
type IssuerValidator struct {
	ExpectedIssuer string
}

// Validate verifies the 'iss' claim against ExpectedIssuer.
func (v *IssuerValidator) Validate(_ context.Context, claims jwt.MapClaims) error {
	if v == nil || strings.TrimSpace(v.ExpectedIssuer) == "" {
		return nil
	}

	issClaim, ok := claims["iss"]
	if !ok {
		return fmt.Errorf("missing 'iss' claim in token")
	}

	issStr, ok := issClaim.(string)
	if !ok || strings.TrimRight(issStr, "/") != strings.TrimRight(v.ExpectedIssuer, "/") {
		return fmt.Errorf("token issuer %q does not match expected issuer %q", issStr, v.ExpectedIssuer)
	}

	return nil
}

// AudienceValidator validates that the token 'aud' matches configured audiences.
// If AllowedAuthorizedParties is also configured, it independently validates the 'azp' claim.
type AudienceValidator struct {
	AllowedAudiences         []string
	AllowedAuthorizedParties []string
}

// Validate verifies that the token audience matches at least one allowed audience.
// If AllowedAuthorizedParties is configured, it also verifies that 'azp' matches.
func (v *AudienceValidator) Validate(_ context.Context, claims jwt.MapClaims) error {
	if v == nil {
		return nil
	}

	if len(v.AllowedAudiences) > 0 {
		audClaim, exists := claims["aud"]
		if !exists || audClaim == nil {
			return fmt.Errorf("missing 'aud' claim in token, expected one of %v", v.AllowedAudiences)
		}

		allowedSet := make(map[string]bool, len(v.AllowedAudiences))
		for _, a := range v.AllowedAudiences {
			allowedSet[a] = true
		}

		if !matchAudienceClaim(audClaim, allowedSet) {
			return fmt.Errorf("token audience does not match any of the allowed audiences %v", v.AllowedAudiences)
		}
	}

	if len(v.AllowedAuthorizedParties) > 0 {
		azpClaim, exists := claims["azp"]
		if !exists || azpClaim == nil {
			return fmt.Errorf("missing 'azp' claim in token, expected one of %v", v.AllowedAuthorizedParties)
		}

		azpSet := make(map[string]bool, len(v.AllowedAuthorizedParties))
		for _, azp := range v.AllowedAuthorizedParties {
			azpSet[azp] = true
		}

		if !matchAzpClaim(azpClaim, azpSet) {
			return fmt.Errorf("token authorized party %v does not match allowed azp %v", azpClaim, v.AllowedAuthorizedParties)
		}
	}

	return nil
}

// MatchAudienceClaim matches the audience claim against allowed audiences.
func matchAudienceClaim(audClaim any, allowedSet map[string]bool) bool {
	if audClaim == nil {
		return false
	}
	if audStr, ok := audClaim.(string); ok {
		return allowedSet[audStr]
	}
	if audList, ok := audClaim.([]string); ok {
		return matchStringSliceInSet(audList, allowedSet)
	}
	if audList, ok := audClaim.([]any); ok {
		return matchAnySliceInSet(audList, allowedSet)
	}
	return false
}

func matchStringSliceInSet(list []string, allowedSet map[string]bool) bool {
	for _, str := range list {
		if allowedSet[str] {
			return true
		}
	}
	return false
}

func matchAnySliceInSet(list []any, allowedSet map[string]bool) bool {
	for _, item := range list {
		if str, ok := item.(string); ok && allowedSet[str] {
			return true
		}
	}
	return false
}

// MatchAzpClaim matches the azp claim against allowed audiences.
func matchAzpClaim(azpClaim any, allowedSet map[string]bool) bool {
	if azpClaim == nil {
		return false
	}
	if azpStr, ok := azpClaim.(string); ok {
		return allowedSet[azpStr]
	}
	return false
}
