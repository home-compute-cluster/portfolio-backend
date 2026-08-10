package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/visitor"
)

func TestCreateCommentReturnsPublicComment(t *testing.T) {
	t.Parallel()

	createdAt := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	service := &fakeCommentService{created: comments.Comment{
		ID:         42,
		PostSlug:   "known-post",
		AuthorName: "Alice",
		Body:       "Hello",
		CreatedAt:  createdAt,
	}}
	response := commentRequest(t, service, http.MethodPost, "/api/posts/known-post/comments", `{
		"author_name":"Alice",
		"body":"Hello",
		"website":""
	}`)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	if service.createCalls != 1 || service.lastCreate.PostSlug != "known-post" {
		t.Fatalf("create calls = %d, input = %#v", service.createCalls, service.lastCreate)
	}
	if service.lastCreate.VisitorHash == ([32]byte{}) {
		t.Fatal("visitor hash was not populated")
	}

	var body map[string]any
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["author_name"] != "Alice" || body["body"] != "Hello" {
		t.Fatalf("response = %#v", body)
	}
	if _, exposed := body["visitor_hash"]; exposed {
		t.Fatal("public response exposed visitor hash")
	}
}

func TestCreateCommentHoneypotSilentlyDropsSubmission(t *testing.T) {
	t.Parallel()

	service := &fakeCommentService{}
	response := commentRequest(t, service, http.MethodPost, "/api/posts/known-post/comments", `{
		"author_name":"bot",
		"body":"spam",
		"website":"https://spam.example"
	}`)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
	if service.createCalls != 0 {
		t.Fatalf("honeypot called service %d times", service.createCalls)
	}
}

func TestCreateCommentRejectsHostileJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want int
	}{
		{"unknown field", `{"author_name":"a","body":"b","admin":true}`, http.StatusBadRequest},
		{"trailing value", `{"author_name":"a","body":"b"}{}`, http.StatusBadRequest},
		{"oversized", `{"author_name":"a","body":"` + strings.Repeat("x", int(maximumCommentRequestBytes)) + `"}`, http.StatusRequestEntityTooLarge},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := commentRequest(t, &fakeCommentService{}, http.MethodPost, "/api/posts/known-post/comments", test.body)
			if response.Code != test.want {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.want, response.Body.String())
			}
		})
	}
}

func TestListCommentsReturnsCursorAndBoundsInput(t *testing.T) {
	t.Parallel()

	service := &fakeCommentService{listed: []comments.Comment{
		{ID: 3, AuthorName: "A", Body: "new", CreatedAt: time.Now()},
		{ID: 2, AuthorName: "B", Body: "old", CreatedAt: time.Now()},
	}}
	response := commentRequest(t, service, http.MethodGet, "/api/posts/known-post/comments?limit=2", "")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	var body commentListResponse
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.NextBeforeID == nil || *body.NextBeforeID != 2 {
		t.Fatalf("next cursor = %v, want 2", body.NextBeforeID)
	}

	response = commentRequest(t, service, http.MethodGet, "/api/posts/known-post/comments?before_id=-1", "")
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid cursor status = %d, want 400", response.Code)
	}
}

func TestCommentErrorsMapWithoutLeakingDetails(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		want int
	}{
		{content.ErrNotFound, http.StatusNotFound},
		{comments.ErrInvalidBody, http.StatusBadRequest},
		{comments.ErrPostFull, http.StatusConflict},
		{errors.New("private database detail"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		service := &fakeCommentService{createErr: test.err}
		response := commentRequest(t, service, http.MethodPost, "/api/posts/known-post/comments", `{"author_name":"a","body":"b"}`)
		if response.Code != test.want {
			t.Fatalf("error %v status = %d, want %d", test.err, response.Code, test.want)
		}
		if strings.Contains(response.Body.String(), "private database detail") {
			t.Fatal("response leaked internal error")
		}
	}
}

func commentRequest(
	t *testing.T,
	service *fakeCommentService,
	method string,
	target string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	handler := NewCommentHandler(
		service,
		visitor.NewIdentity([]byte("0123456789abcdef0123456789abcdef")),
		time.Second,
		logger,
	)
	router := NewApplicationRouter(
		&fakePinger{},
		time.Second,
		logger,
		FeatureHandlers{
			ClientIP: httpmiddleware.NewClientIPResolver(nil),
			Comments: handler,
		},
	)
	request := httptest.NewRequest(method, target, bytes.NewBufferString(body))
	if method == http.MethodPost {
		request.Header.Set("Content-Type", "application/json")
	}
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type fakeCommentService struct {
	created     comments.Comment
	createErr   error
	listed      []comments.Comment
	listErr     error
	createCalls int
	lastCreate  comments.CreateInput
}

func (service *fakeCommentService) Create(_ context.Context, input comments.CreateInput) (comments.Comment, error) {
	service.createCalls++
	service.lastCreate = input
	return service.created, service.createErr
}

func (service *fakeCommentService) ListVisible(context.Context, string, int64, int) ([]comments.Comment, error) {
	return service.listed, service.listErr
}
