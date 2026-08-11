package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"
)

func TestRunExercisesPublicAndProtectedSurfaces(t *testing.T) {
	t.Parallel()

	var mutex sync.Mutex
	called := make(map[string]int)
	handler := http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		mutex.Lock()
		called[request.Method+" "+request.URL.Path]++
		mutex.Unlock()

		if request.URL.Path == "/api/admin/comments" || request.URL.Path == "/api/admin/comments/7/hide" || request.URL.Path == "/api/admin/comments/7/unhide" {
			assertion := request.Header.Get("Cf-Access-Jwt-Assertion")
			if assertion != "valid" {
				response.WriteHeader(http.StatusUnauthorized)
				return
			}
		}
		switch request.Method + " " + request.URL.Path {
		case "GET /api/healthz", "GET /api/readyz", "GET /api/posts/test-post/stats",
			"POST /api/admin/comments/7/hide", "POST /api/admin/comments/7/unhide":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{}`))
		case "GET /api/posts/test-post/comments", "GET /api/admin/comments":
			response.Header().Set("Content-Type", "application/json")
			_, _ = response.Write([]byte(`{"comments":[{"id":7}],"next_before_id":null}`))
		case "POST /api/posts/test-post/view", "PUT /api/posts/test-post/like", "DELETE /api/posts/test-post/like":
			response.WriteHeader(http.StatusNoContent)
		case "POST /api/posts/test-post/comments":
			response.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(response).Encode(map[string]int64{"id": 7})
		default:
			response.WriteHeader(http.StatusNotFound)
		}
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	base, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test URL: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err = run(ctx, smokeConfig{
		publicURL:      base,
		adminOriginURL: base,
		postSlug:       "test-post",
		assertion:      "valid",
	}, server.Client())
	if err != nil {
		t.Fatalf("run smoke workflow: %v", err)
	}
	for _, endpoint := range []string{
		"POST /api/posts/test-post/comments",
		"POST /api/admin/comments/7/hide",
		"POST /api/admin/comments/7/unhide",
	} {
		want := 1
		if endpoint == "POST /api/admin/comments/7/hide" {
			want = 2
		}
		if called[endpoint] != want {
			t.Errorf("%s calls = %d, want %d", endpoint, called[endpoint], want)
		}
	}
	if called["GET /api/admin/comments"] != 3 {
		t.Errorf("GET /api/admin/comments calls = %d, want valid, missing, and forged checks", called["GET /api/admin/comments"])
	}
}

func TestRunFailsWhenAdminOriginDoesNotEnforceAccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost && request.URL.Path == "/api/posts/test-post/comments" {
			response.WriteHeader(http.StatusCreated)
			_, _ = response.Write([]byte(`{"id":7}`))
			return
		}
		if request.Method == http.MethodPost || request.Method == http.MethodPut || request.Method == http.MethodDelete {
			response.WriteHeader(http.StatusNoContent)
			return
		}
		response.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	base, _ := url.Parse(server.URL)

	err := run(context.Background(), smokeConfig{
		publicURL:      base,
		adminOriginURL: base,
		postSlug:       "test-post",
		assertion:      "valid",
	}, server.Client())
	if err == nil {
		t.Fatal("run error = nil, want fail-closed enforcement error")
	}
}
