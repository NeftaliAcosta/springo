package security

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"
)

// JSONWebKey represents a key in a JSON Web Key Set (RFC 7517).
type JSONWebKey struct {
	Kty string   `json:"kty"`
	Use string   `json:"use"`
	Alg string   `json:"alg"`
	Kid string   `json:"kid"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	X5c []string `json:"x5c"`
}

// JSONWebKeySet represents a set of JSONWebKeys.
type JSONWebKeySet struct {
	Keys []JSONWebKey `json:"keys"`
}

// JwksClient manages dynamic JWKS public keys downloading and caching with rate-limited refreshes.
type JwksClient struct {
	jwksURL     string
	keys        map[string]*rsa.PublicKey
	mu          sync.RWMutex
	lastRefresh time.Time
	minInterval time.Duration // Minimum duration to wait between HTTP requests to prevent hammering
	client      *http.Client
}

// NewJwksClient creates a new client with default cache-control settings.
func NewJwksClient(jwksURL string) *JwksClient {
	return &JwksClient{
		jwksURL:     jwksURL,
		keys:        make(map[string]*rsa.PublicKey),
		minInterval: 5 * time.Minute,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetPublicKey retrieves the cached RSA public key for a given key ID (kid).
// If the key is not in cache, it will trigger a fetch from the remote JWKS URL.
func (c *JwksClient) GetPublicKey(kid string) (*rsa.PublicKey, error) {
	c.mu.RLock()
	key, exists := c.keys[kid]
	c.mu.RUnlock()

	if exists {
		return key, nil
	}

	// Double-checked locking to avoid concurrent redundant requests
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check again under write lock
	if key, exists = c.keys[kid]; exists {
		return key, nil
	}

	// Rate limit fetches to prevent network flood on unknown kids
	if time.Since(c.lastRefresh) < c.minInterval {
		return nil, fmt.Errorf("key id '%s' not found and jwks fetch rate-limited (last fetch %v ago)", kid, time.Since(c.lastRefresh))
	}

	if err := c.fetchJWKSLocked(); err != nil {
		return nil, fmt.Errorf("failed to fetch jwks keys from remote: %w", err)
	}

	if key, exists = c.keys[kid]; exists {
		return key, nil
	}

	return nil, fmt.Errorf("key id '%s' not found in jwks even after fetch", kid)
}

func (c *JwksClient) fetchJWKSLocked() error {
	resp, err := c.client.Get(c.jwksURL)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("jwks endpoint returned status code %d", resp.StatusCode)
	}

	var jwks JSONWebKeySet
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return err
	}

	newKeys := make(map[string]*rsa.PublicKey)
	for _, k := range jwks.Keys {
		if k.Kty == "RSA" && k.N != "" && k.E != "" {
			pubKey, err := parseRSAPublicKey(k.N, k.E)
			if err == nil {
				newKeys[k.Kid] = pubKey
			}
		}
	}

	// Replace the cache entirely
	c.keys = newKeys
	c.lastRefresh = time.Now()
	return nil
}

func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	decN, err := base64.RawURLEncoding.DecodeString(nStr)
	if err != nil {
		decN, err = base64.URLEncoding.DecodeString(nStr)
		if err != nil {
			return nil, err
		}
	}

	decE, err := base64.RawURLEncoding.DecodeString(eStr)
	if err != nil {
		decE, err = base64.URLEncoding.DecodeString(eStr)
		if err != nil {
			return nil, err
		}
	}

	var eVal int
	for _, b := range decE {
		eVal = (eVal << 8) | int(b)
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(decN),
		E: eVal,
	}, nil
}
