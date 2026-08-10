package visitor

import (
	"crypto/hmac"
	"crypto/sha256"
	"net/netip"
	"strings"
)

const maximumUserAgentBytes = 512

type Identity struct {
	key []byte
}

func NewIdentity(key []byte) *Identity {
	keyCopy := append([]byte(nil), key...)
	return &Identity{key: keyCopy}
}

func (identity *Identity) Hash(address netip.Addr, userAgent string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, identity.key)
	_, _ = mac.Write([]byte(address.Unmap().String()))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(normalizeUserAgent(userAgent)))

	var result [sha256.Size]byte
	copy(result[:], mac.Sum(nil))
	return result
}

func normalizeUserAgent(value string) string {
	normalized := strings.Join(strings.Fields(value), " ")
	if len(normalized) > maximumUserAgentBytes {
		normalized = normalized[:maximumUserAgentBytes]
	}
	return normalized
}
