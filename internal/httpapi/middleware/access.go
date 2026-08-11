package middleware

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/home-compute-cluster/portfolio-backend/internal/adminauth"
)

const accessAssertionHeader = "Cf-Access-Jwt-Assertion"

type adminPrincipalContextKey struct{}

// AccessVerifier validates a raw Cloudflare Access assertion.
type AccessVerifier interface {
	Verify(ctx context.Context, assertion string) (adminauth.Principal, error)
}

// AccessAuthenticator protects an entire admin route group with Access validation.
type AccessAuthenticator struct {
	verifier AccessVerifier
	logger   *slog.Logger
}

// NewAccessAuthenticator constructs the fail-closed admin authentication middleware.
func NewAccessAuthenticator(verifier AccessVerifier, logger *slog.Logger) *AccessAuthenticator {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &AccessAuthenticator{verifier: verifier, logger: logger}
}

// Authenticate rejects missing or invalid assertions before invoking the protected handler.
func (authenticator *AccessAuthenticator) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		assertion := request.Header.Get(accessAssertionHeader)
		if assertion == "" {
			authenticator.logger.Warn(
				"admin Access assertion rejected",
				"error_category", "access_assertion_missing",
				"request_id", chimiddleware.GetReqID(request.Context()),
			)
			writeUnauthorized(response)
			return
		}
		principal, err := authenticator.verifier.Verify(request.Context(), assertion)
		if err != nil {
			category := "access_assertion_invalid"
			if errors.Is(err, adminauth.ErrSigningKeyUnavailable) {
				category = "access_key_refresh_failed"
			}
			authenticator.logger.Warn(
				"admin Access assertion rejected",
				"error_category", category,
				"request_id", chimiddleware.GetReqID(request.Context()),
				"error", err,
			)
			writeUnauthorized(response)
			return
		}
		ctx := context.WithValue(request.Context(), adminPrincipalContextKey{}, principal)
		next.ServeHTTP(response, request.WithContext(ctx))
	})
}

// AdminPrincipal returns the validated administrator principal from request context.
func AdminPrincipal(ctx context.Context) (adminauth.Principal, bool) {
	principal, ok := ctx.Value(adminPrincipalContextKey{}).(adminauth.Principal)
	return principal, ok && principal.ActorID != ""
}

func writeUnauthorized(response http.ResponseWriter) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(response).Encode(struct {
		Error string `json:"error"`
	}{Error: "unauthorized"})
}
