package adminauth

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestJWKSCacheCachesKnownKeyAndRefreshesUnknownKey(t *testing.T) {
	first := generateKey(t)
	second := generateKey(t)
	var document atomic.Value
	document.Store(jwksDocument(t, "first", first))
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/cdn-cgi/access/certs" {
			http.NotFound(response, request)
			return
		}
		requests.Add(1)
		response.Header().Set("Content-Type", "application/json")
		_, _ = response.Write(document.Load().([]byte))
	}))
	defer server.Close()

	cache := NewJWKSCache(server.URL, server.Client(), time.Hour)
	if _, err := cache.Key(context.Background(), "first"); err != nil {
		t.Fatalf("first Key() error = %v", err)
	}
	if _, err := cache.Key(context.Background(), "first"); err != nil {
		t.Fatalf("cached Key() error = %v", err)
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("JWKS requests after cache hit = %d, want 1", got)
	}

	document.Store(jwksDocument(t, "second", second))
	if _, err := cache.Key(context.Background(), "second"); err != nil {
		t.Fatalf("rotated Key() error = %v", err)
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("JWKS requests after rotation = %d, want 2", got)
	}
}

func TestJWKSCacheRefreshFailureFailsClosed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		http.Error(response, "unavailable", http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cache := NewJWKSCache(server.URL, server.Client(), time.Hour)
	if _, err := cache.Key(context.Background(), "unknown"); err == nil {
		t.Fatal("Key() error = nil during JWKS failure")
	}
}

func jwksDocument(t *testing.T, keyID string, key *rsa.PrivateKey) []byte {
	t.Helper()
	publicKey := &key.PublicKey
	exponent := big.NewInt(int64(publicKey.E)).Bytes()
	document := struct {
		Keys []map[string]string `json:"keys"`
	}{Keys: []map[string]string{{
		"kid": keyID,
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(exponent),
	}}}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal JWKS: %v", err)
	}
	return data
}
