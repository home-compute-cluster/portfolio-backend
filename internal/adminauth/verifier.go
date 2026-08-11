package adminauth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

const (
	maximumAssertionBytes = 16 * 1024
	clockSkew             = 30 * time.Second
)

var (
	ErrMalformedAssertion = errors.New("malformed Access assertion")
	ErrInvalidAssertion   = errors.New("invalid Access assertion")
	ErrUnknownSigningKey  = errors.New("unknown Access signing key")
	// ErrSigningKeyUnavailable distinguishes an operational JWKS refresh failure from a bad token.
	ErrSigningKeyUnavailable = errors.New("access signing keys unavailable")
)

// Principal is the validated, minimal administrator identity passed to handlers.
type Principal struct {
	ActorID string
}

// SigningKeyProvider resolves an Access RSA signing key by JWT key ID.
type SigningKeyProvider interface {
	Key(ctx context.Context, keyID string) (*rsa.PublicKey, error)
}

// Verifier cryptographically validates Cloudflare Access application assertions.
type Verifier struct {
	issuer     string
	audience   string
	adminEmail string
	keys       SigningKeyProvider
	now        func() time.Time
}

// NewVerifier constructs an RS256 Access assertion verifier.
func NewVerifier(
	issuer string,
	audience string,
	adminEmail string,
	keys SigningKeyProvider,
) *Verifier {
	return &Verifier{
		issuer:     issuer,
		audience:   audience,
		adminEmail: strings.ToLower(adminEmail),
		keys:       keys,
		now:        time.Now,
	}
}

// Verify validates signature, issuer, audience, time, and administrator identity.
func (verifier *Verifier) Verify(ctx context.Context, assertion string) (Principal, error) {
	if assertion == "" || len(assertion) > maximumAssertionBytes {
		return Principal{}, ErrMalformedAssertion
	}
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return Principal{}, ErrMalformedAssertion
	}

	headerBytes, err := decodeJWTPart(parts[0])
	if err != nil {
		return Principal{}, ErrMalformedAssertion
	}
	var header struct {
		Algorithm string `json:"alg"`
		KeyID     string `json:"kid"`
	}
	if err := decodeJSON(headerBytes, &header); err != nil || header.Algorithm != "RS256" || header.KeyID == "" {
		return Principal{}, ErrMalformedAssertion
	}

	key, err := verifier.keys.Key(ctx, header.KeyID)
	if err != nil {
		return Principal{}, fmt.Errorf("resolve Access signing key: %w", err)
	}
	signature, err := decodeJWTPart(parts[2])
	if err != nil {
		return Principal{}, ErrMalformedAssertion
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return Principal{}, ErrInvalidAssertion
	}

	payloadBytes, err := decodeJWTPart(parts[1])
	if err != nil {
		return Principal{}, ErrMalformedAssertion
	}
	var claims accessClaims
	if err := decodeJSON(payloadBytes, &claims); err != nil {
		return Principal{}, ErrMalformedAssertion
	}
	if err := verifier.validateClaims(claims); err != nil {
		return Principal{}, err
	}
	return Principal{ActorID: claims.Subject}, nil
}

func (verifier *Verifier) validateClaims(claims accessClaims) error {
	if claims.Type != "app" || claims.Issuer != verifier.issuer || !claims.Audience.Contains(verifier.audience) {
		return ErrInvalidAssertion
	}
	if claims.Subject == "" || !strings.EqualFold(claims.Email, verifier.adminEmail) {
		return ErrInvalidAssertion
	}

	now := verifier.now()
	expiresAt, ok := claims.Expiration.Time()
	if !ok || !expiresAt.After(now.Add(-clockSkew)) {
		return ErrInvalidAssertion
	}
	if notBefore, present := claims.NotBefore.Time(); present && notBefore.After(now.Add(clockSkew)) {
		return ErrInvalidAssertion
	}
	if issuedAt, present := claims.IssuedAt.Time(); present && issuedAt.After(now.Add(clockSkew)) {
		return ErrInvalidAssertion
	}
	return nil
}

type accessClaims struct {
	Type       string      `json:"type"`
	Issuer     string      `json:"iss"`
	Audience   audience    `json:"aud"`
	Subject    string      `json:"sub"`
	Email      string      `json:"email"`
	Expiration numericDate `json:"exp"`
	NotBefore  numericDate `json:"nbf"`
	IssuedAt   numericDate `json:"iat"`
}

type audience []string

func (values *audience) UnmarshalJSON(data []byte) error {
	var one string
	if err := json.Unmarshal(data, &one); err == nil {
		*values = audience{one}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*values = many
	return nil
}

func (values audience) Contains(want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

type numericDate struct {
	seconds int64
	present bool
}

func (date *numericDate) UnmarshalJSON(data []byte) error {
	var value json.Number
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	seconds, err := value.Int64()
	if err != nil {
		return err
	}
	date.seconds = seconds
	date.present = true
	return nil
}

func (date numericDate) Time() (time.Time, bool) {
	if !date.present {
		return time.Time{}, false
	}
	return time.Unix(date.seconds, 0), true
}

func decodeJWTPart(value string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(value)
}

func decodeJSON(data []byte, destination any) error {
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("JWT part contains trailing JSON")
	}
	return nil
}
