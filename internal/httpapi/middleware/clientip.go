package middleware

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

var ErrInvalidRemoteAddress = errors.New("invalid remote address")

type clientIPContextKey struct{}

type ClientIPResolver struct {
	trustedProxies []netip.Prefix
}

func NewClientIPResolver(trustedProxies []netip.Prefix) *ClientIPResolver {
	return &ClientIPResolver{trustedProxies: append([]netip.Prefix(nil), trustedProxies...)}
}

func (resolver *ClientIPResolver) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		address, err := resolver.Resolve(request)
		if err != nil {
			http.Error(response, http.StatusText(http.StatusBadRequest), http.StatusBadRequest)
			return
		}

		ctx := context.WithValue(request.Context(), clientIPContextKey{}, address)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

func (resolver *ClientIPResolver) Resolve(request *http.Request) (netip.Addr, error) {
	peer, err := parseRemoteAddress(request.RemoteAddr)
	if err != nil {
		return netip.Addr{}, err
	}
	if !resolver.isTrusted(peer) {
		return peer, nil
	}

	forwarded := request.Header.Values("X-Forwarded-For")
	if len(forwarded) == 0 {
		return peer, nil
	}

	chain, ok := parseForwardedChain(strings.Join(forwarded, ","))
	if !ok || len(chain) == 0 {
		return peer, nil
	}
	for index := len(chain) - 1; index >= 0; index-- {
		if !resolver.isTrusted(chain[index]) {
			return chain[index], nil
		}
	}

	return chain[0], nil
}

func ClientIP(ctx context.Context) (netip.Addr, bool) {
	address, ok := ctx.Value(clientIPContextKey{}).(netip.Addr)
	return address, ok && address.IsValid()
}

func (resolver *ClientIPResolver) isTrusted(address netip.Addr) bool {
	address = address.Unmap()
	for _, prefix := range resolver.trustedProxies {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}

func parseRemoteAddress(value string) (netip.Addr, error) {
	host, _, err := net.SplitHostPort(value)
	if err != nil {
		return netip.Addr{}, ErrInvalidRemoteAddress
	}
	address, err := netip.ParseAddr(host)
	if err != nil {
		return netip.Addr{}, ErrInvalidRemoteAddress
	}
	return address.Unmap(), nil
}

func parseForwardedChain(value string) ([]netip.Addr, bool) {
	parts := strings.Split(value, ",")
	result := make([]netip.Addr, 0, len(parts))
	for _, part := range parts {
		address, err := netip.ParseAddr(strings.TrimSpace(part))
		if err != nil {
			return nil, false
		}
		result = append(result, address.Unmap())
	}
	return result, true
}
