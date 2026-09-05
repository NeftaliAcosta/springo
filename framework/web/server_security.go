package web

import (
	"fmt"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/security"
)

// Default security header constants.
const (
	DefaultContentTypeOptions    = "nosniff"
	DefaultFrameOptions          = "DENY"
	DefaultXSSProtection         = "0"
	DefaultReferrerPolicy        = "strict-origin-when-cross-origin"
	DefaultPermissionsPolicy     = "camera=(), microphone=(), geolocation=(), payment=()"
	DefaultCrossDomainPolicies   = "none"
	DefaultContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:;"
	DefaultHSTSMaxAge            = 31536000
)

// HSTSProperties defines HTTP Strict Transport Security configuration.
type HSTSProperties struct {
	MaxAge            int   `yaml:"max-age"`
	IncludeSubdomains *bool `yaml:"include-subdomains"`
	Preload           bool  `yaml:"preload"`
}

// Validate normalizes and validates HSTS properties.
func (h *HSTSProperties) Validate() error {
	if h.MaxAge < 0 {
		return fmt.Errorf("server.security.headers.hsts.max-age cannot be negative: %d", h.MaxAge)
	}
	return nil
}

// FormatHeader builds the Strict-Transport-Security header value.
func (h *HSTSProperties) FormatHeader() string {
	maxAge := h.MaxAge
	if maxAge == 0 {
		maxAge = DefaultHSTSMaxAge
	}
	includeSub := true
	if h.IncludeSubdomains != nil {
		includeSub = *h.IncludeSubdomains
	}

	val := fmt.Sprintf("max-age=%d", maxAge)
	if includeSub {
		val += "; includeSubDomains"
	}
	if h.Preload {
		val += "; preload"
	}
	return val
}

// SecurityHeadersProperties defines customizable HTTP response security headers.
type SecurityHeadersProperties struct {
	ContentTypeOptions    string         `yaml:"content-type-options"`
	FrameOptions          string         `yaml:"frame-options"`
	XSSProtection         string         `yaml:"xss-protection"`
	ReferrerPolicy        string         `yaml:"referrer-policy"`
	PermissionsPolicy     string         `yaml:"permissions-policy"`
	CrossDomainPolicies   string         `yaml:"cross-domain-policies"`
	ContentSecurityPolicy string         `yaml:"content-security-policy"`
	HSTS                  HSTSProperties `yaml:"hsts"`
}

// Validate normalizes and validates security header sub-properties.
func (s *SecurityHeadersProperties) Validate() error {
	return s.HSTS.Validate()
}

// ServerSecurityProperties defines HTTP security pipeline controls for the web server.
type ServerSecurityProperties struct {
	CsrfEnabled            *bool                     `yaml:"csrf-enabled"`
	SecurityHeadersEnabled *bool                     `yaml:"security-headers-enabled"`
	CorsEnabled            *bool                     `yaml:"cors-enabled"`
	Headers                SecurityHeadersProperties `yaml:"headers"`
}

// Validate normalizes and validates server security properties.
func (s *ServerSecurityProperties) Validate() error {
	return s.Headers.Validate()
}

// IsCsrfEnabled determines whether CSRF protection middleware should be active.
// Priority order:
// 1. Explicit server.security.csrf-enabled
// 2. Automatic bypass if OAuth2 Resource Server is configured and active
// 3. Explicit spring.security.csrf.enabled (legacy/dedicated config)
// 4. Default: false for stateless REST APIs.
func (s *ServerSecurityProperties) IsCsrfEnabled() bool {
	if s != nil && s.CsrfEnabled != nil {
		return *s.CsrfEnabled
	}

	if oauth2Props := config.Get[security.OAuth2ResourceServerProperties](); oauth2Props != nil && oauth2Props.IsEnabled() {
		return false
	}

	if csrfProps := config.Get[CsrfProperties](); csrfProps != nil {
		return csrfProps.Enabled
	}

	return false
}

// IsSecurityHeadersEnabled determines whether security headers middleware is active (default true).
func (s *ServerSecurityProperties) IsSecurityHeadersEnabled() bool {
	if s != nil && s.SecurityHeadersEnabled != nil {
		return *s.SecurityHeadersEnabled
	}
	return true
}

// IsCorsEnabled determines whether CORS middleware is active (default true).
func (s *ServerSecurityProperties) IsCorsEnabled() bool {
	if s != nil && s.CorsEnabled != nil {
		return *s.CorsEnabled
	}
	return true
}
