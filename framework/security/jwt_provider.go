package security

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JwtProvider handles the creation and validation of JSON Web Tokens (symmetric HS and asymmetric RS).
type JwtProvider struct {
	secret       []byte
	expiration   time.Duration
	jwksURL      string
	publicKeyPEM string
	algorithm    string
	jwksClient   *JwksClient
	publicKeyRSA *rsa.PublicKey
}

// NewJwtProvider initializes the provider with security properties (symmetric key default).
func NewJwtProvider(secret string, expirationMinutes int) *JwtProvider {
	if expirationMinutes == 0 {
		expirationMinutes = 15
	}
	return &JwtProvider{
		secret:     []byte(secret),
		expiration: time.Duration(expirationMinutes) * time.Minute,
		algorithm:  "HS256",
	}
}

// WithAsymmetricConfig configures the provider for asymmetric signature verification.
func (p *JwtProvider) WithAsymmetricConfig(jwksURL, publicKeyPEM, algorithm string) *JwtProvider {
	p.jwksURL = jwksURL
	p.publicKeyPEM = publicKeyPEM
	p.algorithm = algorithm

	if p.algorithm == "" {
		p.algorithm = "HS256"
	}

	if jwksURL != "" {
		p.jwksClient = NewJwksClient(jwksURL)
	}

	if publicKeyPEM != "" {
		p.parseStaticPublicKey(publicKeyPEM)
	}

	return p
}

// ParseStaticPublicKey parses a PEM-encoded public key string.
func (p *JwtProvider) parseStaticPublicKey(pemStr string) {
	pemBytes := parsePEMBlock(pemStr)
	if rsaKey := extractRSAPublicKey(pemBytes); rsaKey != nil {
		p.publicKeyRSA = rsaKey
	}
}

func parsePEMBlock(pemStr string) []byte {
	block, _ := pem.Decode([]byte(pemStr))
	if block != nil {
		return block.Bytes
	}
	return []byte(pemStr)
}

func extractRSAPublicKey(pemBytes []byte) *rsa.PublicKey {
	if pubKey, err := x509.ParsePKIXPublicKey(pemBytes); err == nil {
		if rsaKey, ok := pubKey.(*rsa.PublicKey); ok {
			return rsaKey
		}
	}
	if cert, err := x509.ParseCertificate(pemBytes); err == nil {
		if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			return rsaKey
		}
	}
	return nil
}

// GenerateToken creates a new JWT for a given username/subject and roles.
func (p *JwtProvider) GenerateToken(subject string, roles []string) (string, error) {
	return p.GenerateTokenWithClaims(subject, roles, nil)
}

// GenerateTokenWithClaims creates a new JWT with subject, roles, and custom claims.
func (p *JwtProvider) GenerateTokenWithClaims(
	subject string,
	roles []string,
	customClaims map[string]interface{},
) (string, error) {
	claims := jwt.MapClaims{
		"sub":   subject,
		"roles": roles,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(p.expiration).Unix(),
	}

	for k, v := range customClaims {
		claims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(p.secret)
}

// ValidateToken parses and validates a token string.
func (p *JwtProvider) ValidateToken(tokenString string) (*jwt.Token, error) {
	expectedAlg := p.algorithm
	if expectedAlg == "" {
		expectedAlg = "HS256"
	}

	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return p.resolveKey(token, expectedAlg)
	}, jwt.WithValidMethods([]string{expectedAlg}), jwt.WithExpirationRequired())
}

// ResolveKey resolves the key for a given token.
func (p *JwtProvider) resolveKey(token *jwt.Token, expectedAlg string) (interface{}, error) {
	alg, ok := token.Header["alg"].(string)
	if !ok {
		return nil, fmt.Errorf("missing token algorithm in header")
	}

	if alg != expectedAlg {
		return nil, fmt.Errorf("unexpected signing method: expected %s, got %s", expectedAlg, alg)
	}

	if strings.HasPrefix(alg, "RS") {
		return p.resolveAsymmetricKey(token, alg)
	}

	return p.resolveSymmetricKey(token)
}

// ResolveAsymmetricKey resolves the asymmetric key for a given token.
func (p *JwtProvider) resolveAsymmetricKey(token *jwt.Token, alg string) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("unexpected signing method for asymmetric algorithm: %v", token.Header["alg"])
	}

	if p.jwksClient != nil {
		return p.resolveJwksKey(token)
	}

	if p.publicKeyRSA != nil {
		return p.publicKeyRSA, nil
	}

	return nil, fmt.Errorf("asymmetric algorithm %s is used but no JWKS URL or Public Key is configured", alg)
}

func (p *JwtProvider) resolveJwksKey(token *jwt.Token) (any, error) {
	kid, ok := token.Header["kid"].(string)
	if !ok {
		return nil, fmt.Errorf("missing key id (kid) in token header for RS algorithm")
	}
	return p.jwksClient.GetPublicKey(kid)
}

// ResolveSymmetricKey resolves the symmetric key for a given token.
func (p *JwtProvider) resolveSymmetricKey(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}

	if len(p.secret) == 0 {
		return nil, fmt.Errorf("JWT secret is not configured")
	}

	return p.secret, nil
}

// GetSubjectAndRoles extracts the "sub" and "roles" claims from a valid token.
func (p *JwtProvider) GetSubjectAndRoles(token *jwt.Token) (string, []string, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", nil, fmt.Errorf("invalid token claims")
	}

	subject := ExtractPrincipal(claims, "sub")
	roles := ExtractRoles(claims, "roles", "", "")

	return subject, roles, nil
}

// ExtractPrincipal extracts username using configured principal claim with fallback to "sub".
func ExtractPrincipal(claims jwt.MapClaims, principalClaim string) string {
	if len(claims) == 0 {
		return ""
	}
	if name := extractNamedClaim(claims, principalClaim); name != "" {
		return name
	}
	return extractNamedClaim(claims, "sub")
}

func extractNamedClaim(claims jwt.MapClaims, claimName string) string {
	if claimName == "" {
		return ""
	}
	val, ok := claims[claimName].(string)
	if !ok {
		return ""
	}
	return val
}

// ExtractRoles extracts roles in priority order: direct authorities claim, resource_access, fallback "roles".
func ExtractRoles(claims jwt.MapClaims, authoritiesClaim, resourceID, prefix string) []string {
	var rawRoles []string

	// 1. Direct authorities claim if configured and present.
	if authoritiesClaim != "" {
		rawRoles = append(rawRoles, parseStringSliceClaim(claims[authoritiesClaim])...)
	}

	// 2. Keycloak resource_access client roles.
	if len(rawRoles) == 0 {
		rawRoles = append(rawRoles, extractResourceAccessRoles(claims, resourceID)...)
	}

	// 3. Fallback standard "roles" claim.
	if len(rawRoles) == 0 && authoritiesClaim != "roles" {
		rawRoles = append(rawRoles, parseStringSliceClaim(claims["roles"])...)
	}

	return formatRolesWithPrefix(rawRoles, prefix)
}

// ExtractResourceAccessRoles extracts roles from the "resource_access" claim.
func extractResourceAccessRoles(claims jwt.MapClaims, resourceID string) []string {
	raMap, ok := claims["resource_access"].(map[string]interface{})
	if !ok {
		return nil
	}

	targetClient := getTargetClient(claims, resourceID)
	if targetClient == "" {
		return nil
	}

	clientMap, ok := raMap[targetClient].(map[string]interface{})
	if !ok {
		return nil
	}

	return parseStringSliceClaim(clientMap["roles"])
}

func getTargetClient(claims jwt.MapClaims, resourceID string) string {
	if resourceID != "" {
		return resourceID
	}
	if azp, ok := claims["azp"].(string); ok {
		return azp
	}
	return ""
}

// ParseStringSliceClaim parses a string slice claim from a JWT token.
func parseStringSliceClaim(claimVal any) []string {
	if claimVal == nil {
		return nil
	}
	if str, ok := claimVal.(string); ok && str != "" {
		return []string{str}
	}
	if list, ok := claimVal.([]string); ok {
		return list
	}
	if list, ok := claimVal.([]interface{}); ok {
		return extractStringsFromInterfaceSlice(list)
	}
	return nil
}

func extractStringsFromInterfaceSlice(list []interface{}) []string {
	var res []string
	for _, item := range list {
		if s, ok := item.(string); ok && s != "" {
			res = append(res, s)
		}
	}
	return res
}

// FormatRolesWithPrefix formats roles with a given prefix.
func formatRolesWithPrefix(roles []string, prefix string) []string {
	if len(roles) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(roles))
	var formatted []string
	for _, r := range roles {
		finalRole := formatRoleItem(r, prefix)
		if !seen[finalRole] {
			seen[finalRole] = true
			formatted = append(formatted, finalRole)
		}
	}
	return formatted
}

func formatRoleItem(r, prefix string) string {
	if prefix != "" && !strings.HasPrefix(r, prefix) {
		return prefix + r
	}
	return r
}
