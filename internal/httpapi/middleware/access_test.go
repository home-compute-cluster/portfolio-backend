package middleware

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/home-compute-cluster/portfolio-backend/internal/adminauth"
)

func TestAccessAuthenticatorRejectsMissingAndInvalidAssertions(t *testing.T) {
	t.Parallel()

	for _, test := range []struct {
		name      string
		assertion string
	}{
		{"missing", ""},
		{"invalid", "invalid-token"},
	} {
		t.Run(test.name, func(t *testing.T) {
			verifier := &stubAccessVerifier{err: errors.New("invalid")}
			reached := false
			handler := NewAccessAuthenticator(verifier, nil).Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				reached = true
			}))
			request := httptest.NewRequest(http.MethodGet, "/api/admin/comments", nil)
			if test.assertion != "" {
				request.Header.Set(accessAssertionHeader, test.assertion)
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != http.StatusUnauthorized || reached {
				t.Fatalf("status = %d, reached = %t", response.Code, reached)
			}
			if strings.Contains(response.Body.String(), test.assertion) && test.assertion != "" {
				t.Fatal("unauthorized response exposed assertion")
			}
		})
	}
}

func TestAccessAuthenticatorCategorizesSigningKeyRefreshFailure(t *testing.T) {
	t.Parallel()

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	verifier := &stubAccessVerifier{err: adminauth.ErrSigningKeyUnavailable}
	handler := NewAccessAuthenticator(verifier, logger).Authenticate(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("handler reached during signing-key failure")
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/admin/comments", nil)
	request.Header.Set(accessAssertionHeader, "not-logged")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", response.Code)
	}
	if !strings.Contains(logs.String(), "access_key_refresh_failed") {
		t.Fatalf("log = %s, want refresh failure category", logs.String())
	}
	if strings.Contains(logs.String(), "not-logged") {
		t.Fatal("log exposed Access assertion")
	}
}

func TestAccessAuthenticatorProvidesValidatedPrincipal(t *testing.T) {
	t.Parallel()

	verifier := &stubAccessVerifier{principal: adminauth.Principal{ActorID: "stable-admin-id"}}
	handler := NewAccessAuthenticator(verifier, nil).Authenticate(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		principal, ok := AdminPrincipal(request.Context())
		if !ok || principal.ActorID != "stable-admin-id" {
			t.Fatalf("principal = %#v, ok = %t", principal, ok)
		}
		response.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/api/admin/comments", nil)
	request.Header.Set(accessAssertionHeader, "valid-token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

type stubAccessVerifier struct {
	principal adminauth.Principal
	err       error
}

func (verifier *stubAccessVerifier) Verify(context.Context, string) (adminauth.Principal, error) {
	return verifier.principal, verifier.err
}
