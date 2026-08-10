package visitor

import (
	"net/netip"
	"testing"
)

func TestHashIsStableForNormalizedInputs(t *testing.T) {
	t.Parallel()

	identity := NewIdentity([]byte("0123456789abcdef0123456789abcdef"))
	first := identity.Hash(netip.MustParseAddr("::ffff:192.0.2.1"), "Browser/1.0   Example")
	second := identity.Hash(netip.MustParseAddr("192.0.2.1"), " Browser/1.0 Example ")
	if first != second {
		t.Fatal("equivalent IP and user-agent forms produced different visitor hashes")
	}

	changed := identity.Hash(netip.MustParseAddr("192.0.2.2"), "Browser/1.0 Example")
	if changed == first {
		t.Fatal("different IP produced the same visitor hash")
	}
}

func TestDifferentKeysProduceDifferentHashes(t *testing.T) {
	t.Parallel()

	address := netip.MustParseAddr("192.0.2.1")
	first := NewIdentity([]byte("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")).Hash(address, "agent")
	second := NewIdentity([]byte("bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")).Hash(address, "agent")
	if first == second {
		t.Fatal("different HMAC keys produced the same visitor hash")
	}
}
