package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

func TestRequestLoggerRecordsTemplateAndResponseMetadata(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(RequestLogger(logger))
	router.Get("/items/{id}", func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusTeapot)
		_, _ = response.Write([]byte("no"))
	})

	request := httptest.NewRequest(http.MethodGet, "/items/private-value", nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if event["request_id"] == "" || event["method"] != http.MethodGet || event["route"] != "/items/{id}" {
		t.Fatalf("request fields = %#v", event)
	}
	if event["status"] != float64(http.StatusTeapot) || event["response_bytes"] != float64(2) {
		t.Fatalf("response fields = %#v", event)
	}
	if event["error_category"] != "client_error" {
		t.Fatalf("error category = %#v, want client_error", event["error_category"])
	}
	if bytes.Contains(output.Bytes(), []byte("private-value")) {
		t.Fatal("request log contains a concrete path parameter")
	}
}

func TestRequestLoggerRecordsUnmatchedRoute(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	router := chi.NewRouter()
	router.Use(chimiddleware.RequestID)
	router.Use(RequestLogger(logger))
	router.Handle("/*", http.NotFoundHandler())

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/missing", nil))
	var event map[string]any
	if err := json.Unmarshal(output.Bytes(), &event); err != nil {
		t.Fatalf("decode log: %v", err)
	}
	if event["route"] != "unmatched" || event["status"] != float64(http.StatusNotFound) {
		t.Fatalf("unmatched request fields = %#v", event)
	}
}
