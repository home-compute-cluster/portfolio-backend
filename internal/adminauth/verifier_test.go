package adminauth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestVerifierValidatesAccessAssertion(t *testing.T) {
	key := generateKey(t)
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	verifier := NewVerifier(
		"https://team.cloudflareaccess.com",
		"expected-audience",
		"admin@example.com",
		staticKeys{"key-1": &key.PublicKey},
	)
	verifier.now = func() time.Time { return now }
	assertion := signAssertion(t, key, "key-1", "RS256", map[string]any{
		"iss":                     "https://team.cloudflareaccess.com",
		"aud":                     []string{"another-audience", "expected-audience"},
		"sub":                     "stable-admin-id",
		"email":                   "Admin@Example.com",
		"iat":                     now.Add(-time.Minute).Unix(),
		"nbf":                     now.Add(-time.Minute).Unix(),
		"exp":                     now.Add(time.Hour).Unix(),
		"custom_cloudflare_claim": "allowed",
	})

	principal, err := verifier.Verify(context.Background(), assertion)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if principal.ActorID != "stable-admin-id" {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestVerifierRejectsInvalidAssertions(t *testing.T) {
	key := generateKey(t)
	otherKey := generateKey(t)
	now := time.Date(2026, time.August, 11, 12, 0, 0, 0, time.UTC)
	verifier := NewVerifier(
		"https://team.cloudflareaccess.com",
		"expected-audience",
		"admin@example.com",
		staticKeys{"key-1": &key.PublicKey},
	)
	verifier.now = func() time.Time { return now }

	validClaims := func() map[string]any {
		return map[string]any{
			"iss":   "https://team.cloudflareaccess.com",
			"aud":   "expected-audience",
			"sub":   "stable-admin-id",
			"email": "admin@example.com",
			"iat":   now.Add(-time.Minute).Unix(),
			"exp":   now.Add(time.Hour).Unix(),
		}
	}
	tests := []struct {
		name      string
		algorithm string
		key       *rsa.PrivateKey
		mutate    func(map[string]any)
	}{
		{"invalid signature", "RS256", otherKey, func(map[string]any) {}},
		{"wrong issuer", "RS256", key, func(claims map[string]any) { claims["iss"] = "https://other.cloudflareaccess.com" }},
		{"wrong audience", "RS256", key, func(claims map[string]any) { claims["aud"] = "other-audience" }},
		{"expired", "RS256", key, func(claims map[string]any) { claims["exp"] = now.Add(-time.Minute).Unix() }},
		{"future not-before", "RS256", key, func(claims map[string]any) { claims["nbf"] = now.Add(time.Minute).Unix() }},
		{"future issued-at", "RS256", key, func(claims map[string]any) { claims["iat"] = now.Add(time.Minute).Unix() }},
		{"wrong email", "RS256", key, func(claims map[string]any) { claims["email"] = "other@example.com" }},
		{"missing subject", "RS256", key, func(claims map[string]any) { delete(claims, "sub") }},
		{"wrong algorithm", "HS256", key, func(map[string]any) {}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			claims := validClaims()
			test.mutate(claims)
			assertion := signAssertion(t, test.key, "key-1", test.algorithm, claims)
			if _, err := verifier.Verify(context.Background(), assertion); err == nil {
				t.Fatal("Verify() error = nil, want rejection")
			}
		})
	}
}

func TestVerifierRejectsMalformedAndUnknownKeyAssertions(t *testing.T) {
	key := generateKey(t)
	verifier := NewVerifier("issuer", "audience", "admin@example.com", staticKeys{})
	if _, err := verifier.Verify(context.Background(), "not-a-jwt"); !errors.Is(err, ErrMalformedAssertion) {
		t.Fatalf("malformed error = %v", err)
	}
	assertion := signAssertion(t, key, "unknown", "RS256", map[string]any{})
	if _, err := verifier.Verify(context.Background(), assertion); !errors.Is(err, ErrUnknownSigningKey) {
		t.Fatalf("unknown key error = %v", err)
	}
}

type staticKeys map[string]*rsa.PublicKey

func (keys staticKeys) Key(_ context.Context, keyID string) (*rsa.PublicKey, error) {
	key, exists := keys[keyID]
	if !exists {
		return nil, ErrUnknownSigningKey
	}
	return key, nil
}

func generateKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	return key
}

func signAssertion(
	t *testing.T,
	key *rsa.PrivateKey,
	keyID string,
	algorithm string,
	claims map[string]any,
) string {
	t.Helper()
	headerJSON, err := json.Marshal(map[string]any{"alg": algorithm, "kid": keyID, "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal JWT header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal JWT claims: %v", err)
	}
	header := base64.RawURLEncoding.EncodeToString(headerJSON)
	payload := base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(header + "." + payload))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}
	return header + "." + payload + "." + base64.RawURLEncoding.EncodeToString(signature)
}
