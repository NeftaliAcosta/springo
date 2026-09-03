package security

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestLdapAuthenticationProvider_Disabled(t *testing.T) {
	props := &LdapProperties{Enabled: false}
	provider := NewLdapAuthenticationProvider(props)

	_, _, err := provider.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "disabled")
}

func TestLdapAuthenticationProvider_EmptyCredentials(t *testing.T) {
	props := &LdapProperties{Enabled: true}
	provider := NewLdapAuthenticationProvider(props)

	_, _, err := provider.Authenticate("", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")

	_, _, err = provider.Authenticate("user", "")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be empty")
}

func TestLdapAuthenticationProvider_MissingUrls(t *testing.T) {
	props := &LdapProperties{Enabled: true}
	provider := NewLdapAuthenticationProvider(props)

	_, _, err := provider.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not configured")
}

func TestLdapAuthenticationProvider_ConnectionFailure(t *testing.T) {
	props := &LdapProperties{
		Enabled: true,
		Urls:    "ldap://127.0.0.1:9999", // Invalid port
	}
	provider := NewLdapAuthenticationProvider(props)

	_, _, err := provider.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to connect")
}

func TestLdapAuthenticationProvider_RequireTLS(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock TCP server: %v", err)
	}
	defer func() { _ = l.Close() }()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
			}(conn)
		}
	}()

	addr := "ldap://" + l.Addr().String()

	// Case 1: RequireTLS is true -> Must fail when StartTLS fails
	props := &LdapProperties{
		Enabled:    true,
		Urls:       addr,
		StartTLS:   true,
		RequireTLS: true,
	}
	provider := NewLdapAuthenticationProvider(props)
	_, _, err = provider.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires secure StartTLS, but TLS handshake failed")

	// Case 2: Production profile -> Must fail when StartTLS fails (even if RequireTLS is false)
	t.Setenv("SPRINGO_PROFILES_ACTIVE", "prod")

	propsProd := &LdapProperties{
		Enabled:    true,
		Urls:       addr,
		StartTLS:   true,
		RequireTLS: false, // will be overridden to true in prod profile
	}
	providerProd := NewLdapAuthenticationProvider(propsProd)
	_, _, err = providerProd.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires secure StartTLS, but TLS handshake failed")
}

func TestLdapAuthenticationProvider_ExtraTLSScenarios(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start mock TCP server: %v", err)
	}
	defer func() { _ = l.Close() }()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
			}(conn)
		}
	}()

	addr := "ldap://" + l.Addr().String()

	// Scenario 1: profile = "production" (alternative prod profile name) with StartTLS failure
	t.Setenv("SPRINGO_PROFILES_ACTIVE", "production")
	propsProd2 := &LdapProperties{
		Enabled:    true,
		Urls:       addr,
		StartTLS:   true,
		RequireTLS: false,
	}
	providerProd2 := NewLdapAuthenticationProvider(propsProd2)
	_, _, err = providerProd2.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires secure StartTLS, but TLS handshake failed")

	// Scenario 2: profile = "prod" with StartTLS = false -> Must fail immediately since TLS is required
	t.Setenv("SPRINGO_PROFILES_ACTIVE", "prod")
	propsNoStartTLSProd := &LdapProperties{
		Enabled:    true,
		Urls:       addr,
		StartTLS:   false,
		RequireTLS: false,
	}
	providerNoStartTLSProd := NewLdapAuthenticationProvider(propsNoStartTLSProd)
	_, _, err = providerNoStartTLSProd.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires secure TLS in production, but StartTLS is disabled")

	// Scenario 3: Non-production profile with StartTLS = false -> Proceed with warning (should not fail with TLS errors, but rather fail on search because mock server doesn't respond)
	t.Setenv("SPRINGO_PROFILES_ACTIVE", "dev")
	propsDev := &LdapProperties{
		Enabled:          true,
		Urls:             addr,
		StartTLS:         false,
		RequireTLS:       false,
		UserSearchFilter: "(uid={0})",
	}
	providerDev := NewLdapAuthenticationProvider(propsDev)
	_, _, err = providerDev.Authenticate("user", "pass")
	assert.Error(t, err)
	// The call proceeds past TLS checks and fails client-side (no response to bind/search packets)
	assert.NotContains(t, err.Error(), "requires secure StartTLS")
	assert.NotContains(t, err.Error(), "requires secure TLS in production")

	t.Setenv("SPRINGO_PROFILES_ACTIVE", "")
}

func TestLdapAuthenticationProvider_InvalidSchemes(t *testing.T) {
	props := &LdapProperties{
		Enabled: true,
		Urls:    "http://localhost:80",
	}
	provider := NewLdapAuthenticationProvider(props)
	_, _, err := provider.Authenticate("user", "pass")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported LDAP scheme")
}

func generateSelfSignedCert() (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"SprinGo Test"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	privBytes, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return tls.Certificate{}, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: derBytes})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})

	return tls.X509KeyPair(certPEM, keyPEM)
}

func TestLdapAuthenticationProvider_MockLdapsConnection(t *testing.T) {
	cert, err := generateSelfSignedCert()
	if err != nil {
		t.Fatalf("failed to generate self-signed cert: %v", err)
	}

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}
	l, err := tls.Listen("tcp", "127.0.0.1:0", tlsConfig)
	if err != nil {
		t.Fatalf("failed to start mock TLS listener: %v", err)
	}
	defer func() { _ = l.Close() }()

	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				buf := make([]byte, 1024)
				_, _ = c.Read(buf)
			}(conn)
		}
	}()

	addr := "ldaps://" + l.Addr().String()

	props := &LdapProperties{
		Enabled:          true,
		Urls:             addr,
		TLSSkipVerify:    true,
		UserSearchFilter: "(uid={0})",
	}
	provider := NewLdapAuthenticationProvider(props)

	// Authenticate should complete the TLS handshake successfully, and then fail on search/bind.
	_, _, err = provider.Authenticate("user", "pass")
	assert.Error(t, err)
	// It should NOT fail on the handshake or connection Dial itself, but rather proceed to the LDAP protocol
	assert.NotContains(t, err.Error(), "handshake failed")
	assert.NotContains(t, err.Error(), "failed to connect")
}
