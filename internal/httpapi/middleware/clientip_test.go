package middleware

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIPIgnoresForwardedHeaderFromUntrustedPeer(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "198.51.100.10:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.7")
	resolver := NewClientIPResolver([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})

	got, err := resolver.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := netip.MustParseAddr("198.51.100.10"); got != want {
		t.Fatalf("Resolve() = %s, want direct peer %s", got, want)
	}
}

func TestClientIPWalksTrustedProxyChainRightToLeft(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "10.0.0.3:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.1, 10.0.0.2")
	resolver := NewClientIPResolver([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})

	got, err := resolver.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := netip.MustParseAddr("203.0.113.7"); got != want {
		t.Fatalf("Resolve() = %s, want client %s", got, want)
	}
}

func TestClientIPFallsBackWhenForwardedChainIsMalformed(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "10.0.0.3:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.7, forged")
	resolver := NewClientIPResolver([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})

	got, err := resolver.Resolve(request)
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if want := netip.MustParseAddr("10.0.0.3"); got != want {
		t.Fatalf("Resolve() = %s, want trusted peer fallback %s", got, want)
	}
}

func FuzzClientIPResolver(f *testing.F) {
	f.Add("203.0.113.7, 10.0.0.2")
	f.Add("malformed")
	f.Add("")

	resolver := NewClientIPResolver([]netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")})
	f.Fuzz(func(t *testing.T, forwarded string) {
		request := httptest.NewRequest("GET", "/", nil)
		request.RemoteAddr = "10.0.0.3:1234"
		request.Header.Set("X-Forwarded-For", forwarded)
		_, _ = resolver.Resolve(request)
	})
}
