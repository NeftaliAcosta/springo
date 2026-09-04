package security

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/config"
)

// OAuth2ResourceServerProperties defines configuration for an OAuth2 / OIDC resource server.
// @ConfigurationProperties(prefix="security.oauth2.resourceserver")
type OAuth2ResourceServerProperties struct {
	Enabled          *bool    `yaml:"enabled"`           // Master switch for authentication
	IssuerURI        string   `yaml:"issuer-uri"`        // OIDC provider issuer URL
	JwksURI          string   `yaml:"jwks-uri"`          // JWKS certificates endpoint URL
	Algorithm        string   `yaml:"algorithm"`         // "RS256" (default) or "HS256"
	Secret           string   `yaml:"secret"`            // Secret key when algorithm is HS256
	PublicKey        string   `yaml:"public-key"`        // Static PEM public key fallback
	PrincipalClaim   string   `yaml:"principal-claim"`   // Claim used as authenticated username
	AuthoritiesClaim string   `yaml:"authorities-claim"` // Claim containing direct roles
	ResourceID       string   `yaml:"resource-id"`       // Key inside resource_access for OIDC client roles
	AuthorityPrefix  string   `yaml:"authority-prefix"`  // Prefix applied to extracted roles (default "ROLE_")
	PublicPaths      []string `yaml:"public-paths"`      // Routes exempted from authentication
	Audience         []string `yaml:"audience"`          // Allowed audience values (aud / azp)
	Expiration       int      `yaml:"expiration"`        // Token expiration minutes for issued tokens
}

// IsEnabled returns true if OAuth2 resource server authentication is active.
func (p *OAuth2ResourceServerProperties) IsEnabled() bool {
	if p == nil || p.Enabled == nil {
		return true
	}
	return *p.Enabled
}

// GetAlgorithm returns normalized uppercase algorithm (default "RS256").
func (p *OAuth2ResourceServerProperties) GetAlgorithm() string {
	if p == nil || strings.TrimSpace(p.Algorithm) == "" {
		return "RS256"
	}
	return strings.ToUpper(strings.TrimSpace(p.Algorithm))
}

// GetAuthorityPrefix returns the configured prefix or defaults to "ROLE_".
func (p *OAuth2ResourceServerProperties) GetAuthorityPrefix() string {
	if p == nil || p.AuthorityPrefix == "" {
		return "ROLE_"
	}
	return p.AuthorityPrefix
}

// GetPrincipalClaim returns the principal claim or defaults to "preferred_username".
func (p *OAuth2ResourceServerProperties) GetPrincipalClaim() string {
	if p == nil || p.PrincipalClaim == "" {
		return "preferred_username"
	}
	return p.PrincipalClaim
}

// Validate ensures OAuth2 configuration integrity for production environments.
func (p *OAuth2ResourceServerProperties) Validate() error {
	if !p.IsEnabled() {
		return nil
	}

	alg := p.GetAlgorithm()
	if alg != "RS256" && alg != "HS256" {
		return fmt.Errorf("unsupported OAuth2 algorithm %q (supported: RS256, HS256)", p.Algorithm)
	}

	profile := strings.ToLower(strings.TrimSpace(os.Getenv("SPRINGO_PROFILES_ACTIVE")))
	if isDevelopmentProfile(profile) {
		p.warnIfDevSecret(alg)
		return nil
	}

	return p.validateProduction(alg)
}

func (p *OAuth2ResourceServerProperties) warnIfDevSecret(alg string) {
	if alg == "HS256" && isDevSecret(p.Secret) {
		slog.Warn("Using default development OAuth2 secret. Set SPRINGO_PROFILES_ACTIVE=prod for production")
	}
}

func isDevelopmentProfile(profile string) bool {
	switch profile {
	case "", "default", "dev", "development", "local", "test":
		return true
	default:
		return false
	}
}

func (p *OAuth2ResourceServerProperties) validateProduction(alg string) error {
	if alg == "RS256" {
		return p.validateRS256Production()
	}
	return p.validateHS256Production()
}

func (p *OAuth2ResourceServerProperties) validateRS256Production() error {
	if p.JwksURI == "" && p.IssuerURI == "" && p.PublicKey == "" {
		return fmt.Errorf("OAuth2 algorithm RS256 requires 'jwks-uri', 'issuer-uri' or 'public-key' in prod")
	}
	return nil
}

func (p *OAuth2ResourceServerProperties) validateHS256Production() error {
	if isDevSecret(p.Secret) {
		return fmt.Errorf("OAuth2 secret is insecure, empty, or uses development defaults in prod profile")
	}
	if len(p.Secret) < 32 {
		return fmt.Errorf("OAuth2 secret must be at least 32 characters (256 bits) for production deployment")
	}
	return nil
}

func isDevSecret(secret string) bool {
	return secret == "" || secret == "default-secret" || secret == "springo-ultra-secret-key-for-development"
}

func init() {
	defaultEnabled := true
	config.RegisterProperties("security.oauth2.resourceserver", &OAuth2ResourceServerProperties{
		Enabled:         &defaultEnabled,
		Algorithm:       "RS256",
		AuthorityPrefix: "ROLE_",
		PrincipalClaim:  "preferred_username",
	})
}
