package security

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

const (
	defaultJwksTTL         = 1 * time.Hour
	defaultMinBackoff      = 5 * time.Second
	defaultHTTPTimeout     = 10 * time.Second
	maxJwksPayloadBytes    = 1024 * 1024 // 1MB payload limit
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
	keysMu      sync.RWMutex
	fetchMu     sync.Mutex
	lastSuccess time.Time
	lastAttempt time.Time
	ttl         time.Duration
	minInterval time.Duration
	client      *http.Client
}

// NewJwksClient creates a new client with default cache-control settings.
func NewJwksClient(jwksURL string) *JwksClient {
	return &JwksClient{
		jwksURL:     jwksURL,
		keys:        make(map[string]*rsa.PublicKey),
		ttl:         defaultJwksTTL,
		minInterval: defaultMinBackoff,
		client: &http.Client{
			Timeout: defaultHTTPTimeout,
		},
	}
}

// GetPublicKey retrieves the cached RSA public key for a given key ID (kid).
func (c *JwksClient) GetPublicKey(kid string) (*rsa.PublicKey, error) {
	return c.GetPublicKeyWithContext(context.Background(), kid)
}

// GetPublicKeyWithContext retrieves the cached RSA public key with context support.
func (c *JwksClient) GetPublicKeyWithContext(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	// 1. Fast path: check current cache without blocking other readers
	c.keysMu.RLock()
	key, exists := c.keys[kid]
	fresh := time.Since(c.lastSuccess) < c.ttl
	c.keysMu.RUnlock()

	if exists && fresh {
		return key, nil
	}

	// 2. Slow path: single-flight refresh under fetch lock
	c.fetchMu.Lock()
	defer c.fetchMu.Unlock()

	// Re-check cache under fetch lock
	c.keysMu.RLock()
	key, exists = c.keys[kid]
	fresh = time.Since(c.lastSuccess) < c.ttl
	c.keysMu.RUnlock()

	if exists && fresh {
		return key, nil
	}

	// Rate-limit backoff on repeated failure attempts
	if time.Since(c.lastAttempt) < c.minInterval {
		if exists {
			return key, nil // Return stale key if available during failure window
		}
		return nil, fmt.Errorf("key id %q not found and jwks fetch rate-limited (last attempt %v ago)",
			kid, time.Since(c.lastAttempt))
	}

	c.lastAttempt = time.Now()

	// 3. Fetch remote keys outside the keysMu read/write lock
	newKeys, err := c.fetchRemoteKeys(ctx)
	if err != nil {
		if exists {
			return key, nil // Fallback to existing stale key on network failure
		}
		return nil, fmt.Errorf("failed to fetch jwks keys from remote: %w", err)
	}

	// 4. Update cached snapshot
	c.keysMu.Lock()
	c.keys = newKeys
	c.lastSuccess = time.Now()
	key, exists = c.keys[kid]
	c.keysMu.Unlock()

	if !exists {
		return nil, fmt.Errorf("key id %q not found in jwks even after fetch", kid)
	}

	return key, nil
}

func (c *JwksClient) fetchRemoteKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.jwksURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks endpoint returned status code %d", resp.StatusCode)
	}

	// Limit reader to prevent unbounded memory allocation
	limitReader := io.LimitReader(resp.Body, maxJwksPayloadBytes)
	var jwks JSONWebKeySet
	if err := json.NewDecoder(limitReader).Decode(&jwks); err != nil {
		return nil, err
	}

	newKeys := make(map[string]*rsa.PublicKey, len(jwks.Keys))
	for _, k := range jwks.Keys {
		if k.Kty == "RSA" && k.N != "" && k.E != "" {
			pubKey, err := parseRSAPublicKey(k.N, k.E)
			if err == nil {
				newKeys[k.Kid] = pubKey
			}
		}
	}

	return newKeys, nil
}

func parseRSAPublicKey(nStr, eStr string) (*rsa.PublicKey, error) {
	decN, err := decodeBase64URL(nStr)
	if err != nil {
		return nil, err
	}

	decE, err := decodeBase64URL(eStr)
	if err != nil {
		return nil, err
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

func decodeBase64URL(s string) ([]byte, error) {
	decoded, err := base64.RawURLEncoding.DecodeString(s)
	if err == nil {
		return decoded, nil
	}
	return base64.URLEncoding.DecodeString(s)
}
