package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/ratelimit"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/visitor"
	"github.com/home-compute-cluster/portfolio-backend/internal/reactions"
)

func TestLikeEndpointsUseExplicitDesiredState(t *testing.T) {
	t.Parallel()

	service := &fakeReactionService{}
	for _, test := range []struct {
		method string
		want   string
	}{
		{http.MethodPut, "like"},
		{http.MethodDelete, "unlike"},
	} {
		response := reactionRequest(t, service, nil, test.method, "/api/posts/known-post/like")
		if response.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204", test.method, response.Code)
		}
		if service.lastOperation != test.want || service.slug != "known-post" {
			t.Fatalf("%s operation = %q, slug = %q", test.method, service.lastOperation, service.slug)
		}
		if service.visitorHash == ([32]byte{}) {
			t.Fatalf("%s did not receive visitor hash", test.method)
		}
	}
}

func TestStatsEndpointReturnsOnlyPublicTotals(t *testing.T) {
	t.Parallel()

	service := &fakeReactionService{stats: reactions.Stats{Views: 123, Likes: 17}}
	// Stats is a public read and remains usable without client-IP middleware.
	response := reactionRequest(t, service, nil, http.MethodGet, "/api/posts/known-post/stats")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["views"] != float64(123) || body["likes"] != float64(17) || len(body) != 2 {
		t.Fatalf("stats response = %#v", body)
	}
}

func TestReactionEndpointsMapUnknownPost(t *testing.T) {
	t.Parallel()

	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		service := &fakeReactionService{err: content.ErrNotFound}
		path := "/api/posts/unknown-post/stats"
		if method != http.MethodGet {
			path = "/api/posts/unknown-post/like"
		}
		response := reactionRequest(t, service, nil, method, path)
		if response.Code != http.StatusNotFound {
			t.Fatalf("%s status = %d, want 404", method, response.Code)
		}
	}
}

func TestReactionErrorsDoNotLeakInternals(t *testing.T) {
	t.Parallel()

	service := &fakeReactionService{err: errors.New("private database detail")}
	response := reactionRequest(t, service, nil, http.MethodGet, "/api/posts/known-post/stats")
	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", response.Code)
	}
	if strings.Contains(response.Body.String(), "private database detail") {
		t.Fatal("reaction response leaked internal error")
	}
}

func TestLikeRateLimitBoundaryReturns429(t *testing.T) {
	t.Parallel()

	service := &fakeReactionService{}
	response := reactionRequest(t, service, denyingLimiter{}, http.MethodPut, "/api/posts/known-post/like")
	if response.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429", response.Code)
	}
	if service.lastOperation != "" {
		t.Fatal("rate-limited request reached like service")
	}
}

func reactionRequest(
	t *testing.T,
	service *fakeReactionService,
	limiter ratelimit.Limiter,
	method string,
	path string,
) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewReactionHandler(
		service,
		service,
		visitor.NewIdentity([]byte("0123456789abcdef0123456789abcdef")),
		limiter,
		time.Second,
		nil,
	)
	features := FeatureHandlers{Reactions: handler}
	if method != http.MethodGet {
		features.ClientIP = httpmiddleware.NewClientIPResolver(nil)
	}
	router := NewApplicationRouter(&fakePinger{}, time.Second, nil, features)
	request := httptest.NewRequest(method, path, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type fakeReactionService struct {
	stats         reactions.Stats
	err           error
	lastOperation string
	slug          string
	visitorHash   [32]byte
}

func (service *fakeReactionService) Like(
	_ context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	service.lastOperation = "like"
	service.slug = postSlug
	service.visitorHash = visitorHash
	return true, service.err
}

func (service *fakeReactionService) Unlike(
	_ context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	service.lastOperation = "unlike"
	service.slug = postSlug
	service.visitorHash = visitorHash
	return true, service.err
}

func (service *fakeReactionService) Get(
	_ context.Context,
	postSlug string,
) (reactions.Stats, error) {
	service.slug = postSlug
	return service.stats, service.err
}
