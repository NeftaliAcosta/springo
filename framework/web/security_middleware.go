package web

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"strings"

	"github.com/NeftaliAcosta/springo/framework/config"
	"github.com/NeftaliAcosta/springo/framework/errors"
	"github.com/NeftaliAcosta/springo/framework/security"

	"github.com/golang-jwt/jwt/v5"
)

// IsActuatorPath checks if the current request path matches the actuator endpoint or subpaths.
func isActuatorPath(path string) bool {
	p := strings.TrimSuffix(path, "/")
	return p == "/actuator" || strings.HasPrefix(path, "/actuator/")
}

// SecurityConfig holds configuration for the security authentication middleware.
type securityConfig struct {
	enabled          bool
	publicPaths      []string
	principalClaim   string
	authoritiesClaim string
	resourceID       string
	authorityPrefix  string
	provider         *security.JwtProvider
	validators       []security.TokenValidator
}

// ResolveSecurityConfig resolves the security configuration from the framework properties.
func resolveSecurityConfig() securityConfig {
	if oauth2Props := config.Get[security.OAuth2ResourceServerProperties](); isOAuth2Configured(oauth2Props) {
		return buildOAuth2Config(oauth2Props)
	}

	if jwtProps := config.Get[security.JwtProperties](); isLegacyJwtConfigured(jwtProps) {
		return buildLegacyJwtConfig(jwtProps)
	}

	return defaultSecurityConfig()
}

func isOAuth2Configured(props *security.OAuth2ResourceServerProperties) bool {
	if props == nil {
		return false
	}
	if props.Enabled != nil && !*props.Enabled {
		return true
	}
	return hasOAuth2KeysOrURIs(props)
}

func hasOAuth2KeysOrURIs(props *security.OAuth2ResourceServerProperties) bool {
	return props.IssuerURI != "" || props.JwksURI != "" || props.Secret != "" || props.PublicKey != ""
}

func isLegacyJwtConfigured(props *security.JwtProperties) bool {
	if props == nil {
		return false
	}
	return props.Secret != "" || props.JwksURL != "" || props.PublicKey != ""
}

func defaultSecurityConfig() securityConfig {
	provider := security.NewJwtProvider("default-secret", 15)
	return securityConfig{
		enabled:         true,
		publicPaths:     []string{"/swagger"},
		principalClaim:  "sub",
		authorityPrefix: "ROLE_",
		provider:        provider,
	}
}

// BuildOAuth2Config constructs securityConfig from OAuth2 resource server properties.
func buildOAuth2Config(props *security.OAuth2ResourceServerProperties) securityConfig {
	secret := resolveOAuth2Secret(props)
	jwksURL := resolveOAuth2JwksURL(props)

	provider := security.NewJwtProvider(secret, props.Expiration).
		WithAsymmetricConfig(jwksURL, props.PublicKey, props.GetAlgorithm())

	return securityConfig{
		enabled:          props.IsEnabled(),
		publicPaths:      props.PublicPaths,
		principalClaim:   props.GetPrincipalClaim(),
		authoritiesClaim: props.AuthoritiesClaim,
		resourceID:       props.ResourceID,
		authorityPrefix:  props.GetAuthorityPrefix(),
		provider:         provider,
		validators:       buildBuiltInValidators(props),
	}
}

func resolveOAuth2Secret(props *security.OAuth2ResourceServerProperties) string {
	if props.Secret == "" && props.GetAlgorithm() == "HS256" {
		return "default-secret"
	}
	return props.Secret
}

func resolveOAuth2JwksURL(props *security.OAuth2ResourceServerProperties) string {
	if props.JwksURI != "" {
		return props.JwksURI
	}
	if props.IssuerURI != "" {
		return strings.TrimRight(props.IssuerURI, "/") + "/protocol/openid-connect/certs"
	}
	return ""
}

func buildBuiltInValidators(props *security.OAuth2ResourceServerProperties) []security.TokenValidator {
	var valList []security.TokenValidator
	if props.IssuerURI != "" {
		valList = append(valList, &security.IssuerValidator{ExpectedIssuer: props.IssuerURI})
	}
	if len(props.Audience) > 0 {
		valList = append(valList, &security.AudienceValidator{AllowedAudiences: props.Audience})
	}
	return valList
}

// BuildLegacyJwtConfig constructs securityConfig from legacy JWT properties.
func buildLegacyJwtConfig(props *security.JwtProperties) securityConfig {
	secret := props.Secret
	if secret == "" && (props.Algorithm == "" || props.Algorithm == "HS256") {
		secret = "default-secret"
	}

	provider := security.NewJwtProvider(secret, props.Expiration).
		WithAsymmetricConfig(props.JwksURL, props.PublicKey, props.Algorithm)

	return securityConfig{
		enabled:         true,
		publicPaths:     props.PublicPaths,
		principalClaim:  "sub",
		authorityPrefix: "ROLE_",
		provider:        provider,
	}
}

// AuthMiddleware creates a middleware that validates JWT tokens based on framework properties.
func AuthMiddleware(next http.Handler) http.Handler {
	cfg := resolveSecurityConfig()
	if !cfg.enabled {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serveAuthenticated(w, r, cfg, next)
	})
}

func serveAuthenticated(w http.ResponseWriter, r *http.Request, cfg securityConfig, next http.Handler) {
	if isActuatorPath(r.URL.Path) || isPublicPath(r.URL.Path, cfg.publicPaths) {
		next.ServeHTTP(w, r)
		return
	}

	ctx, authErr := authenticateRequest(r, cfg)
	if authErr != nil {
		HandleError(w, r, authErr)
		return
	}

	next.ServeHTTP(w, r.WithContext(ctx))
}

func authenticateRequest(r *http.Request, cfg securityConfig) (context.Context, error) {
	tokenStr, err := extractToken(r)
	if err != nil {
		return nil, errors.Unauthorized(err.Error(), "AUTH_HEADER_ERROR")
	}

	claims, tokenErr := extractAndValidateTokenClaims(tokenStr, cfg)
	if tokenErr != nil {
		return nil, tokenErr
	}

	if valErr := validateClaims(r.Context(), claims, cfg.validators); valErr != nil {
		return nil, errors.Unauthorized(valErr.Error(), "AUTH_VALIDATION_ERROR")
	}

	return buildAuthenticatedContext(r.Context(), claims, tokenStr, cfg), nil
}

func extractAndValidateTokenClaims(tokenStr string, cfg securityConfig) (jwt.MapClaims, error) {
	token, err := cfg.provider.ValidateToken(tokenStr)
	if err != nil || !token.Valid {
		return nil, errors.Unauthorized("invalid or expired token", "AUTH_INVALID_TOKEN")
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, errors.Unauthorized("invalid token claims", "AUTH_INVALID_CLAIMS")
	}
	return claims, nil
}

func buildAuthenticatedContext(
	ctx context.Context,
	claims jwt.MapClaims,
	tokenStr string,
	cfg securityConfig,
) context.Context {
	subject := security.ExtractPrincipal(claims, cfg.principalClaim)
	roles := security.ExtractRoles(claims, cfg.authoritiesClaim, cfg.resourceID, cfg.authorityPrefix)

	secCtx := security.WithSecurityContext(ctx, subject, roles, claims, tokenStr)
	return enrichDynamicClaims(secCtx, claims)
}

// ValidateClaims validates claims with built-in and registry validators.
func validateClaims(ctx context.Context, claims jwt.MapClaims, builtIn []security.TokenValidator) error {
	for _, v := range builtIn {
		if err := v.Validate(ctx, claims); err != nil {
			return err
		}
	}
	for _, v := range security.GetTokenValidators() {
		if err := v.Validate(ctx, claims); err != nil {
			return err
		}
	}
	return nil
}

// EnrichDynamicClaims populates dynamic context keys from claims.
func enrichDynamicClaims(ctx context.Context, claims jwt.MapClaims) context.Context {
	for k, v := range claims {
		ctx = context.WithValue(ctx, k, v) //nolint:staticcheck // Dynamic string keys for legacy claims propagation
		if k == "sub" {
			ctx = context.WithValue(ctx, "username", v) //nolint:staticcheck // Framework conveniency key
			ctx = context.WithValue(ctx, "user", v)     //nolint:staticcheck // Framework conveniency key
		}
	}
	return ctx
}

// SecurityHeadersMiddleware adds basic security headers to every response.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	cspHeader := "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:;"
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "0")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		w.Header().Set("Content-Security-Policy", cspHeader)

		if shouldApplyHSTS(r) {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

func shouldApplyHSTS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return r.Header.Get("X-Forwarded-Proto") == "https" && isHeaderFromTrustedProxy(r)
}

// IsHeaderFromTrustedProxy checks if the header is from a trusted proxy.
func isHeaderFromTrustedProxy(r *http.Request) bool {
	host := extractHostIP(r.RemoteAddr)
	if host == "" {
		return false
	}

	ipAddr, err := netip.ParseAddr(host)
	if err != nil {
		return false
	}

	return isIPTrusted(host, ipAddr)
}

func extractHostIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil && host != "" {
		return host
	}
	return remoteAddr
}

func isIPTrusted(host string, ipAddr netip.Addr) bool {
	props := config.Get[WebServerProperties]()
	if props == nil || len(props.TrustedProxies) == 0 {
		return false
	}

	for _, cidr := range props.TrustedProxies {
		if matchesTrustedCIDR(cidr, host, ipAddr) {
			return true
		}
	}
	return false
}

func matchesTrustedCIDR(cidr, host string, ipAddr netip.Addr) bool {
	if cidr == host {
		return true
	}
	prefix, err := netip.ParsePrefix(cidr)
	return err == nil && prefix.Contains(ipAddr)
}

// IsPublicPath checks if the current request path matches any of the configured public paths.
func isPublicPath(currentPath string, publicPaths []string) bool {
	cur := normalizePath(currentPath)
	for _, path := range publicPaths {
		p := normalizePath(path)
		if matchWildcardPath(cur, p) {
			return true
		}
	}
	return false
}

func normalizePath(path string) string {
	if len(path) > 1 && strings.HasSuffix(path, "/") {
		return strings.TrimSuffix(path, "/")
	}
	return path
}

// MatchWildcardPath matches a wildcard path.
func matchWildcardPath(cur, pattern string) bool {
	if pattern == "/" {
		return cur == "/"
	}
	if strings.HasSuffix(pattern, "/**") {
		return matchDoubleWildcard(cur, pattern)
	}
	if strings.HasSuffix(pattern, "/*") {
		return matchSingleWildcard(cur, pattern)
	}
	return cur == pattern || strings.HasPrefix(cur, pattern+"/")
}

func matchDoubleWildcard(cur, pattern string) bool {
	base := strings.TrimSuffix(pattern, "/**")
	if base == "" || base == "/" {
		return true
	}
	return cur == base || strings.HasPrefix(cur, base+"/")
}

func matchSingleWildcard(cur, pattern string) bool {
	base := strings.TrimSuffix(pattern, "/*")
	if base == "" || base == "/" {
		parts := strings.Split(strings.TrimPrefix(cur, "/"), "/")
		return len(parts) == 1
	}
	return strings.HasPrefix(cur, base+"/")
}

// ExtractToken extracts the JWT token from the authorization header.
func extractToken(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return "", fmt.Errorf("missing authorization header")
	}

	parts := strings.Split(authHeader, " ")
	if len(parts) != 2 || parts[0] != "Bearer" {
		return "", fmt.Errorf("invalid authorization format")
	}
	return parts[1], nil
}
