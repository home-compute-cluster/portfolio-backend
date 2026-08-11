package middleware

import (
	"bytes"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRecovererHidesPanicDetails(t *testing.T) {
	const privatePanic = "private panic detail"

	var logs bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	handler := Recoverer(logger)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic(privatePanic)
	}))

	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	if strings.Contains(response.Body.String(), privatePanic) {
		t.Fatal("response leaked panic detail")
	}
	if response.Header().Get("Content-Type") != "application/json; charset=utf-8" || response.Body.String() != "{\"error\":\"internal_error\"}\n" {
		t.Fatalf("panic response = headers %#v body %q", response.Header(), response.Body.String())
	}
	if !strings.Contains(logs.String(), "panic recovered") {
		t.Fatal("panic was not logged")
	}
	if strings.Contains(logs.String(), privatePanic) {
		t.Fatal("log leaked the recovered panic value")
	}
}
