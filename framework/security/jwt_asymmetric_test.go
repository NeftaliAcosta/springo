package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestJwtProvider_SymmetricHS256(t *testing.T) {
	secret := "my-ultra-secure-symmetric-shared-key-32chars"
	provider := NewJwtProvider(secret, 15)

	// Test generation and validation of standard HS256 token
	tokenStr, err := provider.GenerateToken("test-user", []string{"USER", "ADMIN"})
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}

	token, err := provider.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("Failed to validate token: %v", err)
	}

	if !token.Valid {
		t.Errorf("Expected token to be valid")
	}

	sub, roles, err := provider.GetSubjectAndRoles(token)
	if err != nil {
		t.Fatalf("Failed to get subject and roles: %v", err)
	}

	if sub != "test-user" {
		t.Errorf("Expected subject 'test-user', got '%s'", sub)
	}

	if len(roles) != 2 || roles[0] != "USER" || roles[1] != "ADMIN" {
		t.Errorf("Expected roles ['USER', 'ADMIN'], got %v", roles)
	}
}

func TestJwtProvider_AsymmetricStaticPEM(t *testing.T) {
	// Generate dynamic RSA key pair for testing
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA private key: %v", err)
	}

	// Format public key as PKIX PEM
	pubASN1, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to marshal RSA public key: %v", err)
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})

	// Configure provider with asymmetric static PEM public key
	provider := NewJwtProvider("", 15).
		WithAsymmetricConfig("", string(pubPEM), "RS256")

	// Generate RS256 token using private key
	claims := jwt.MapClaims{
		"sub":   "asymmetric-pem-user",
		"roles": []interface{}{"MANAGER"},
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "static-key-1"

	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("Failed to sign token with private key: %v", err)
	}

	// Validate using provider
	validatedToken, err := provider.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("Failed to validate RS256 token: %v", err)
	}

	if !validatedToken.Valid {
		t.Errorf("Expected token to be valid")
	}

	sub, roles, err := provider.GetSubjectAndRoles(validatedToken)
	if err != nil {
		t.Fatalf("Failed to extract claims: %v", err)
	}

	if sub != "asymmetric-pem-user" {
		t.Errorf("Expected subject 'asymmetric-pem-user', got '%s'", sub)
	}

	if len(roles) != 1 || roles[0] != "MANAGER" {
		t.Errorf("Expected roles ['MANAGER'], got %v", roles)
	}
}

func TestJwtProvider_AsymmetricDynamicJWKS(t *testing.T) {
	// Generate dynamic RSA key pair for testing
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA private key: %v", err)
	}

	kid := "test-key-id-123"

	// Mock JWKS endpoint returning the public key
	jwksHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nStr := base64.RawURLEncoding.EncodeToString(privateKey.N.Bytes())
		eStr := base64.RawURLEncoding.EncodeToString([]byte{1, 0, 1}) // 65537

		jwks := JSONWebKeySet{
			Keys: []JSONWebKey{
				{
					Kty: "RSA",
					Use: "sig",
					Alg: "RS256",
					Kid: kid,
					N:   nStr,
					E:   eStr,
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(jwks)
	})

	server := httptest.NewServer(jwksHandler)
	defer server.Close()

	// Configure provider with JWKS URL
	provider := NewJwtProvider("", 15).
		WithAsymmetricConfig(server.URL, "", "RS256")

	// Generate RS256 token
	claims := jwt.MapClaims{
		"sub":   "jwks-user",
		"roles": []interface{}{"ADMIN"},
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = kid

	tokenStr, err := token.SignedString(privateKey)
	if err != nil {
		t.Fatalf("Failed to sign token with private key: %v", err)
	}

	// Validate using provider (which triggers HTTP call to JWKS Mock)
	validatedToken, err := provider.ValidateToken(tokenStr)
	if err != nil {
		t.Fatalf("Failed to validate RS256 JWKS token: %v", err)
	}

	if !validatedToken.Valid {
		t.Errorf("Expected token to be valid")
	}

	sub, roles, err := provider.GetSubjectAndRoles(validatedToken)
	if err != nil {
		t.Fatalf("Failed to extract claims: %v", err)
	}

	if sub != "jwks-user" {
		t.Errorf("Expected subject 'jwks-user', got '%s'", sub)
	}

	if len(roles) != 1 || roles[0] != "ADMIN" {
		t.Errorf("Expected roles ['ADMIN'], got %v", roles)
	}
}

func TestJwtProvider_AlgConfusionAttackSafety(t *testing.T) {
	// Generate keys
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA private key: %v", err)
	}

	pubASN1, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to marshal RSA public key: %v", err)
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})

	// Configure provider to expect RS256 with static public key
	provider := NewJwtProvider("", 15).
		WithAsymmetricConfig("", string(pubPEM), "RS256")

	// Attack vector: Sign with HS256 using the Public Key PEM content as the shared symmetric secret!
	claims := jwt.MapClaims{
		"sub":   "attacker",
		"roles": []interface{}{"ADMIN"},
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Signed using HS256 but with the PUBLIC KEY string as secret
	attackTokenStr, err := token.SignedString(pubPEM)
	if err != nil {
		t.Fatalf("Failed to create attack token: %v", err)
	}

	// Validation must strictly fail because signature algorithm is HS256 but provider expects RS
	_, err = provider.ValidateToken(attackTokenStr)
	if err == nil {
		t.Fatalf("Security Vulnerability: Validated token using HS256 while expecting RS256 (Algorithm Confusion)")
	}

	expectedErrorMsg := "unexpected signing method"
	errStr := fmt.Sprintf("%v", err)
	if !strings.Contains(errStr, expectedErrorMsg) && !strings.Contains(errStr, "unexpected signing method for asymmetric algorithm") && !strings.Contains(errStr, "signing method") && !strings.Contains(errStr, "invalid") {
		t.Errorf("Expected error message containing '%s' or valid method rejection, got: %v", expectedErrorMsg, err)
	}
}

func TestJwtProvider_HS384_HS512(t *testing.T) {
	secret := "my-ultra-secure-symmetric-shared-key-64chars-long-long-long-long"

	p384 := NewJwtProvider(secret, 15)
	p384.algorithm = "HS384"

	claims384 := jwt.MapClaims{
		"sub":   "test-384",
		"roles": []interface{}{"USER"},
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	}
	token384 := jwt.NewWithClaims(jwt.SigningMethodHS384, claims384)
	tokenStr384, err := token384.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign HS384 token: %v", err)
	}

	val384, err := p384.ValidateToken(tokenStr384)
	if err != nil {
		t.Fatalf("Failed to validate HS384 token: %v", err)
	}
	if !val384.Valid {
		t.Errorf("HS384 token is invalid")
	}

	p512 := NewJwtProvider(secret, 15)
	p512.algorithm = "HS512"

	claims512 := jwt.MapClaims{
		"sub":   "test-512",
		"roles": []interface{}{"ADMIN"},
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	}
	token512 := jwt.NewWithClaims(jwt.SigningMethodHS512, claims512)
	tokenStr512, err := token512.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("Failed to sign HS512 token: %v", err)
	}

	val512, err := p512.ValidateToken(tokenStr512)
	if err != nil {
		t.Fatalf("Failed to validate HS512 token: %v", err)
	}
	if !val512.Valid {
		t.Errorf("HS512 token is invalid")
	}
}

func TestJwtProvider_RS384_RS512(t *testing.T) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA private key: %v", err)
	}

	pubASN1, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("Failed to marshal RSA public key: %v", err)
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubASN1,
	})

	p384 := NewJwtProvider("", 15).
		WithAsymmetricConfig("", string(pubPEM), "RS384")

	claims384 := jwt.MapClaims{
		"sub":   "test-rs384",
		"roles": []interface{}{"USER"},
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	}
	token384 := jwt.NewWithClaims(jwt.SigningMethodRS384, claims384)
	tokenStr384, err := token384.SignedString(privateKey)
	if err != nil {
		t.Fatalf("Failed to sign RS384 token: %v", err)
	}

	val384, err := p384.ValidateToken(tokenStr384)
	if err != nil {
		t.Fatalf("Failed to validate RS384 token: %v", err)
	}
	if !val384.Valid {
		t.Errorf("RS384 token is invalid")
	}

	p512 := NewJwtProvider("", 15).
		WithAsymmetricConfig("", string(pubPEM), "RS512")

	claims512 := jwt.MapClaims{
		"sub":   "test-rs512",
		"roles": []interface{}{"ADMIN"},
		"iat":   time.Now().Unix(),
		"exp":   time.Now().Add(15 * time.Minute).Unix(),
	}
	token512 := jwt.NewWithClaims(jwt.SigningMethodRS512, claims512)
	tokenStr512, err := token512.SignedString(privateKey)
	if err != nil {
		t.Fatalf("Failed to sign RS512 token: %v", err)
	}

	val512, err := p512.ValidateToken(tokenStr512)
	if err != nil {
		t.Fatalf("Failed to validate RS512 token: %v", err)
	}
	if !val512.Valid {
		t.Errorf("RS512 token is invalid")
	}
}
