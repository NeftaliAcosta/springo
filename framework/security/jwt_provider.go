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

// JwtProvider handles the creation and validation of JSON Web Tokens (supporting symmetric HS and asymmetric RS algorithms)
type JwtProvider struct {
	secret       []byte
	expiration   time.Duration
	jwksURL      string
	publicKeyPEM string
	algorithm    string
	jwksClient   *JwksClient
	publicKeyRSA *rsa.PublicKey
}

// NewJwtProvider initializes the provider with security properties (symmetric key default)
func NewJwtProvider(secret string, expirationMinutes int) *JwtProvider {
	return &JwtProvider{
		secret:     []byte(secret),
		expiration: time.Duration(expirationMinutes) * time.Minute,
		algorithm:  "HS256",
	}
}

// WithAsymmetricConfig configures the provider for asymmetric signature verification
func (p *JwtProvider) WithAsymmetricConfig(jwksURL string, publicKeyPEM string, algorithm string) *JwtProvider {
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

func (p *JwtProvider) parseStaticPublicKey(pemStr string) {
	block, _ := pem.Decode([]byte(pemStr))
	var pemBytes []byte
	if block != nil {
		pemBytes = block.Bytes
	} else {
		pemBytes = []byte(pemStr)
	}

	// Try PKIX public key parsing
	if pubKey, err := x509.ParsePKIXPublicKey(pemBytes); err == nil {
		if rsaKey, ok := pubKey.(*rsa.PublicKey); ok {
			p.publicKeyRSA = rsaKey
			return
		}
	}

	// Fallback to parsing a raw X509 certificate
	if cert, err := x509.ParseCertificate(pemBytes); err == nil {
		if rsaKey, ok := cert.PublicKey.(*rsa.PublicKey); ok {
			p.publicKeyRSA = rsaKey
			return
		}
	}
}

// GenerateToken creates a new JWT for a given username/subject and roles (symmetric only for self-issued tokens)
func (p *JwtProvider) GenerateToken(subject string, roles []string) (string, error) {
	return p.GenerateTokenWithClaims(subject, roles, nil)
}

// GenerateTokenWithClaims creates a new JWT with subject, roles, and custom claims
func (p *JwtProvider) GenerateTokenWithClaims(subject string, roles []string, customClaims map[string]interface{}) (string, error) {
	claims := jwt.MapClaims{
		"sub":   subject,
		"roles": roles,
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(p.expiration).Unix(),
	}

	reservedClaims := map[string]bool{
		"sub": true, "roles": true, "iat": true, "exp": true,
		"nbf": true, "iss": true, "aud": true, "jti": true,
	}

	for k, v := range customClaims {
		if reservedClaims[k] {
			return "", fmt.Errorf("custom claim '%s' is reserved and cannot be overwritten", k)
		}
		claims[k] = v
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(p.secret)
}

// ValidateToken parses and validates a token string
func (p *JwtProvider) ValidateToken(tokenString string) (*jwt.Token, error) {
	expectedAlg := p.algorithm
	if expectedAlg == "" {
		expectedAlg = "HS256"
	}

	return jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		return p.resolveKey(token, expectedAlg)
	}, jwt.WithValidMethods([]string{expectedAlg}))
}

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

func (p *JwtProvider) resolveAsymmetricKey(token *jwt.Token, alg string) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("unexpected signing method for asymmetric algorithm: %v", token.Header["alg"])
	}

	// JWKS key resolving
	if p.jwksClient != nil {
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing key id (kid) in token header for RS algorithm")
		}
		return p.jwksClient.GetPublicKey(kid)
	}

	// Static RSA key resolving
	if p.publicKeyRSA != nil {
		return p.publicKeyRSA, nil
	}

	return nil, fmt.Errorf("asymmetric algorithm %s is used but no JWKS URL or Public Key is configured", alg)
}

func (p *JwtProvider) resolveSymmetricKey(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
		return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
	}

	if len(p.secret) == 0 {
		return nil, fmt.Errorf("JWT secret is not configured")
	}

	return p.secret, nil
}

// GetSubjectAndRoles extracts the "sub" and "roles" claims from a valid token
func (p *JwtProvider) GetSubjectAndRoles(token *jwt.Token) (string, []string, error) {
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return "", nil, fmt.Errorf("invalid token claims")
	}

	subject, _ := claims["sub"].(string)
	roles := extractRolesFromClaims(claims)

	return subject, roles, nil
}

func extractRolesFromClaims(claims jwt.MapClaims) []string {
	var roles []string
	rolesClaim, exists := claims["roles"]
	if !exists {
		return roles
	}

	rolesArray, ok := rolesClaim.([]interface{})
	if !ok {
		return roles
	}

	for _, r := range rolesArray {
		if rStr, ok := r.(string); ok {
			roles = append(roles, rStr)
		}
	}
	return roles
}
