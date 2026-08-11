package adminauth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"sync"
	"time"
)

const maximumJWKSBytes int64 = 1024 * 1024

// JWKSCache fetches and caches Cloudflare Access RSA signing keys.
type JWKSCache struct {
	endpoint string
	client   *http.Client
	ttl      time.Duration
	now      func() time.Time

	mu        sync.Mutex
	keys      map[string]*rsa.PublicKey
	fetchedAt time.Time
}

// NewJWKSCache constructs a rotation-aware cache for the team Access certificates endpoint.
func NewJWKSCache(teamDomain string, client *http.Client, ttl time.Duration) *JWKSCache {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &JWKSCache{
		endpoint: teamDomain + "/cdn-cgi/access/certs",
		client:   client,
		ttl:      ttl,
		now:      time.Now,
		keys:     make(map[string]*rsa.PublicKey),
	}
}

// Key returns a cached key or refreshes the complete JWKS when stale or unknown.
func (cache *JWKSCache) Key(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	if keyID == "" || len(keyID) > 256 {
		return nil, ErrUnknownSigningKey
	}

	cache.mu.Lock()
	defer cache.mu.Unlock()
	key, exists := cache.keys[keyID]
	if exists && cache.now().Sub(cache.fetchedAt) < cache.ttl {
		return key, nil
	}
	if err := cache.refresh(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrSigningKeyUnavailable, err)
	}
	key, exists = cache.keys[keyID]
	if !exists {
		return nil, ErrUnknownSigningKey
	}
	return key, nil
}

func (cache *JWKSCache) refresh(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cache.endpoint, nil)
	if err != nil {
		return fmt.Errorf("build Access JWKS request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := cache.client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch Access JWKS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch Access JWKS: unexpected status %d", response.StatusCode)
	}

	limited := io.LimitReader(response.Body, maximumJWKSBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return fmt.Errorf("read Access JWKS: %w", err)
	}
	if int64(len(data)) > maximumJWKSBytes {
		return errors.New("access JWKS response is too large")
	}
	var document struct {
		Keys []jwk `json:"keys"`
	}
	if err := json.Unmarshal(data, &document); err != nil {
		return fmt.Errorf("decode Access JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, encoded := range document.Keys {
		key, err := encoded.RSAKey()
		if err != nil {
			return fmt.Errorf("decode Access JWK: %w", err)
		}
		if _, duplicate := keys[encoded.KeyID]; duplicate {
			return errors.New("access JWKS contains a duplicate key ID")
		}
		keys[encoded.KeyID] = key
	}
	if len(keys) == 0 {
		return errors.New("access JWKS contains no usable keys")
	}
	cache.keys = keys
	cache.fetchedAt = cache.now()
	return nil
}

type jwk struct {
	KeyID     string `json:"kid"`
	KeyType   string `json:"kty"`
	Algorithm string `json:"alg"`
	Use       string `json:"use"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func (encoded jwk) RSAKey() (*rsa.PublicKey, error) {
	if encoded.KeyID == "" || encoded.KeyType != "RSA" || encoded.Algorithm != "RS256" || encoded.Use != "sig" {
		return nil, errors.New("unsupported Access signing key")
	}
	modulusBytes, err := base64.RawURLEncoding.DecodeString(encoded.Modulus)
	if err != nil {
		return nil, errors.New("invalid RSA modulus")
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(encoded.Exponent)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, errors.New("invalid RSA exponent")
	}
	exponent := 0
	for _, value := range exponentBytes {
		exponent = exponent<<8 | int(value)
	}
	modulus := new(big.Int).SetBytes(modulusBytes)
	if modulus.BitLen() < 2048 || exponent < 3 || exponent%2 == 0 {
		return nil, errors.New("weak or invalid RSA key")
	}
	return &rsa.PublicKey{N: modulus, E: exponent}, nil
}
