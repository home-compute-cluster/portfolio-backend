package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

type fakePinger struct {
	err   error
	calls int
}

func (p *fakePinger) Ping(context.Context) error {
	p.calls++
	return p.err
}

func TestHealthzReturnsOKWithoutPingingDatabase(t *testing.T) {
	database := &fakePinger{err: errors.New("database should not be called")}
	response := serveRequest(t, NewRouter(database, time.Second, nil), "/api/healthz")

	assertStatusResponse(t, response, http.StatusOK, "ok")
	if database.calls != 0 {
		t.Fatalf("expected no database pings, got %d", database.calls)
	}
}

func TestReadyzReturnsReadyWhenDatabaseIsAvailable(t *testing.T) {
	database := &fakePinger{}
	response := serveRequest(t, NewRouter(database, time.Second, nil), "/api/readyz")

	assertStatusResponse(t, response, http.StatusOK, "ready")
	if database.calls != 1 {
		t.Fatalf("expected one database ping, got %d", database.calls)
	}
}

func TestReadyzReturnsUnavailableWithoutLeakingDatabaseError(t *testing.T) {
	const privateError = "private database detail"

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	database := &fakePinger{err: errors.New(privateError)}
	response := serveRequest(t, NewRouter(database, time.Second, logger), "/api/readyz")

	assertStatusResponse(t, response, http.StatusServiceUnavailable, "unavailable")
	if strings.Contains(response.Body.String(), privateError) {
		t.Fatal("readiness response leaked the database error")
	}
	if !strings.Contains(logs.String(), privateError) {
		t.Fatal("readiness failure was not logged internally")
	}
}

func TestRouterReturnsNotFoundForUnknownRoute(t *testing.T) {
	response := serveRequest(t, NewRouter(&fakePinger{}, time.Second, nil), "/api/unknown")

	if response.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.Code)
	}
}

func TestRouterRecoversFromPanics(t *testing.T) {
	handler := newRouter(
		&fakePinger{},
		time.Second,
		nil,
		func(router chi.Router) {
			router.Get("/panic", func(http.ResponseWriter, *http.Request) {
				panic("test panic")
			})
		},
	)

	response := serveRequest(t, handler, "/panic")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("expected status %d, got %d", http.StatusInternalServerError, response.Code)
	}
}

func serveRequest(t *testing.T, handler http.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	return response
}

func assertStatusResponse(
	t *testing.T,
	response *httptest.ResponseRecorder,
	wantCode int,
	wantStatus string,
) {
	t.Helper()

	if response.Code != wantCode {
		t.Fatalf("expected status %d, got %d", wantCode, response.Code)
	}

	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Status != wantStatus {
		t.Fatalf("expected response status %q, got %q", wantStatus, body.Status)
	}
	if contentType := response.Header().Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("unexpected Content-Type %q", contentType)
	}
}
