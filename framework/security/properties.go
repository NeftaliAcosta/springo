package security

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/config"
)

// JwtProperties holds the security configuration for tokens
// @ConfigurationProperties(prefix="security.jwt")
type JwtProperties struct {
	Secret      string   `yaml:"secret"`       // HMAC secret key for HS256
	Expiration  int      `yaml:"expiration"`   // Expiration in minutes (default is 15)
	PublicPaths []string `yaml:"public-paths"` // Paths that don't require authentication
	JwksURL     string   `yaml:"jwks-url"`     // OIDC JWKS endpoint URL for dynamic key fetching
	PublicKey   string   `yaml:"public-key"`   // PEM public key content (optional static fallback)
	Algorithm   string   `yaml:"algorithm"`    // "HS256" (default) or "RS256"
}

// Validate ensures that JWT configuration is secure for production environments.
func (p *JwtProperties) Validate() error {
	alg := strings.ToUpper(strings.TrimSpace(p.Algorithm))
	if alg == "" {
		alg = "HS256"
	}

	if alg != "HS256" && alg != "RS256" {
		return fmt.Errorf("unsupported JWT algorithm %q (supported: HS256, RS256)", p.Algorithm)
	}

	profile := strings.ToLower(strings.TrimSpace(os.Getenv("SPRINGO_PROFILES_ACTIVE")))
	isDevProfile := profile == "" || profile == "default" || profile == "dev" ||
		profile == "development" || profile == "local" || profile == "test"
	if isDevProfile {
		if p.Secret == "springo-ultra-secret-key-for-development" || p.Secret == "default-secret" {
			slog.Warn("Using default development JWT secret. Set SPRINGO_PROFILES_ACTIVE=prod for production")
		}
		return nil
	}

	if alg == "RS256" {
		if p.JwksURL == "" && p.PublicKey == "" {
			return fmt.Errorf("JWT algorithm RS256 requires either 'jwks-url' or 'public-key' in production profile")
		}
		return nil
	}

	// Default HS256 validation
	if p.Secret == "" || p.Secret == "default-secret" || p.Secret == "springo-ultra-secret-key-for-development" {
		return fmt.Errorf("JWT secret is insecure, empty, or uses development defaults in production profile")
	}
	if len(p.Secret) < 32 {
		return fmt.Errorf("JWT secret must be at least 32 characters (256 bits) for production deployment")
	}
	return nil
}

func init() {
	// Register the struct to be filled from the "security.jwt" YAML block
	config.RegisterProperties("security.jwt", &JwtProperties{})
}
