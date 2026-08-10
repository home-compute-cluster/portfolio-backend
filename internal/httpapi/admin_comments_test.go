package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
)

func TestDormantModerationHandlerUsesDesiredStateOperations(t *testing.T) {
	t.Parallel()

	service := &fakeAdminCommentService{}
	handler := NewAdminCommentHandler(service, time.Second, nil)
	router := chi.NewRouter()
	// Test-only registration. The production router intentionally has no admin routes.
	router.Post("/api/admin/comments/{id}/hide", handler.Hide)
	router.Post("/api/admin/comments/{id}/unhide", handler.Unhide)

	for _, operation := range []string{"hide", "unhide"} {
		request := httptest.NewRequest(http.MethodPost, "/api/admin/comments/42/"+operation, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", operation, response.Code)
		}
	}
	if service.hiddenID != 42 || service.unhiddenID != 42 {
		t.Fatalf("desired-state IDs = hide %d, unhide %d", service.hiddenID, service.unhiddenID)
	}
}

func TestDormantModerationHandlerListsStatus(t *testing.T) {
	t.Parallel()

	service := &fakeAdminCommentService{listed: []comments.Comment{{ID: 4, Status: comments.StatusHidden}}}
	handler := NewAdminCommentHandler(service, time.Second, nil)
	request := httptest.NewRequest(http.MethodGet, "/?status=hidden", nil)
	response := httptest.NewRecorder()
	handler.List(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", response.Code)
	}
	if service.status != comments.StatusHidden {
		t.Fatalf("status filter = %q, want hidden", service.status)
	}
	var body struct {
		Comments []moderationCommentResponse `json:"comments"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(body.Comments) != 1 || body.Comments[0].Status != comments.StatusHidden {
		t.Fatalf("moderation response = %#v", body)
	}
}

func TestProductionRouterDoesNotExposeModeration(t *testing.T) {
	t.Parallel()

	response := serveRequest(t, NewRouter(&fakePinger{}, time.Second, nil), "/api/admin/comments")
	if response.Code != http.StatusNotFound {
		t.Fatalf("dormant moderation route status = %d, want 404", response.Code)
	}
}

type fakeAdminCommentService struct {
	listed     []comments.Comment
	status     comments.Status
	hiddenID   int64
	unhiddenID int64
}

func (service *fakeAdminCommentService) List(
	_ context.Context,
	status comments.Status,
	_ int64,
	_ int,
) ([]comments.Comment, error) {
	service.status = status
	return service.listed, nil
}

func (service *fakeAdminCommentService) Hide(_ context.Context, id int64) (comments.Comment, error) {
	service.hiddenID = id
	return comments.Comment{ID: id, Status: comments.StatusHidden}, nil
}

func (service *fakeAdminCommentService) Unhide(_ context.Context, id int64) (comments.Comment, error) {
	service.unhiddenID = id
	return comments.Comment{ID: id, Status: comments.StatusVisible}, nil
}
