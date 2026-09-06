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
	if p.isEmpty() {
		return nil
	}

	alg, err := p.normalizeAlgorithm()
	if err != nil {
		return err
	}

	if isDevProfile() {
		p.warnIfDevSecret()
		return nil
	}

	if alg == "RS256" {
		return p.validateRS256()
	}

	return p.validateHS256()
}

func (p *JwtProperties) normalizeAlgorithm() (string, error) {
	alg := strings.ToUpper(strings.TrimSpace(p.Algorithm))
	if alg == "" {
		return "HS256", nil
	}
	if alg != "HS256" && alg != "RS256" {
		return "", fmt.Errorf("unsupported JWT algorithm %q (supported: HS256, RS256)", p.Algorithm)
	}
	return alg, nil
}

func isDevProfile() bool {
	profile := strings.ToLower(strings.TrimSpace(os.Getenv("SPRINGO_PROFILES_ACTIVE")))
	switch profile {
	case "", "default", "dev", "development", "local", "test":
		return true
	default:
		return false
	}
}

func (p *JwtProperties) warnIfDevSecret() {
	if p.Secret == "springo-ultra-secret-key-for-development" || p.Secret == "default-secret" {
		slog.Warn("Using default development JWT secret. Set SPRINGO_PROFILES_ACTIVE=prod for production")
	}
}

func (p *JwtProperties) validateRS256() error {
	if p.JwksURL == "" && p.PublicKey == "" {
		return fmt.Errorf("JWT algorithm RS256 requires either 'jwks-url' or 'public-key' in production profile")
	}
	return nil
}

func (p *JwtProperties) validateHS256() error {
	if p.Secret == "" || p.Secret == "default-secret" || p.Secret == "springo-ultra-secret-key-for-development" {
		return fmt.Errorf("JWT secret is insecure, empty, or uses development defaults in production profile")
	}
	if len(p.Secret) < 32 {
		return fmt.Errorf("JWT secret must be at least 32 characters (256 bits) for production deployment")
	}
	return nil
}

func (p *JwtProperties) isEmpty() bool {
	return p.Secret == "" && p.JwksURL == "" && p.PublicKey == "" &&
		len(p.PublicPaths) == 0 && p.Expiration == 0 && strings.TrimSpace(p.Algorithm) == ""
}

func init() {
	// Register the struct to be filled from the "security.jwt" YAML block
	config.RegisterProperties("security.jwt", &JwtProperties{})
}
