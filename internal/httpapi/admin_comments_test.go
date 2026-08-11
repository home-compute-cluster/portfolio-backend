package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/adminauth"
	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
)

func TestProtectedModerationHandlerUsesDesiredStateOperations(t *testing.T) {
	t.Parallel()

	service := &fakeAdminCommentService{}
	router := protectedAdminRouter(service)

	for _, operation := range []string{"hide", "unhide"} {
		request := httptest.NewRequest(http.MethodPost, "/api/admin/comments/42/"+operation, nil)
		request.Header.Set("Cf-Access-Jwt-Assertion", "valid-test-assertion")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", operation, response.Code)
		}
	}
	if service.hiddenID != 42 || service.unhiddenID != 42 {
		t.Fatalf("desired-state IDs = hide %d, unhide %d", service.hiddenID, service.unhiddenID)
	}
	if service.audit.ActorID != "access-subject-123" || service.audit.RequestID == "" {
		t.Fatalf("audit context = %#v, want stable actor and request ID", service.audit)
	}
}

func TestProtectedModerationHandlerListsStatus(t *testing.T) {
	t.Parallel()

	service := &fakeAdminCommentService{listed: []comments.Comment{{ID: 4, Status: comments.StatusHidden}}}
	router := protectedAdminRouter(service)
	request := httptest.NewRequest(http.MethodGet, "/api/admin/comments?status=hidden", nil)
	request.Header.Set("Cf-Access-Jwt-Assertion", "valid-test-assertion")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
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

func TestAdminCommentRoutesFailClosedWithoutAccessMiddleware(t *testing.T) {
	t.Parallel()

	response := serveRequest(t, NewApplicationRouter(
		&fakePinger{},
		time.Second,
		nil,
		FeatureHandlers{AdminComments: NewAdminCommentHandler(&fakeAdminCommentService{}, time.Second, nil)},
	), "/api/admin/comments")
	if response.Code != http.StatusNotFound {
		t.Fatalf("unprotected moderation route status = %d, want 404", response.Code)
	}
}

func TestEveryAdminCommentRouteRequiresAccessAssertion(t *testing.T) {
	t.Parallel()

	service := &fakeAdminCommentService{}
	router := protectedAdminRouter(service)
	tests := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/api/admin/comments"},
		{http.MethodPost, "/api/admin/comments/1/hide"},
		{http.MethodPost, "/api/admin/comments/1/unhide"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Errorf("%s %s status = %d, want 401", test.method, test.path, response.Code)
		}
	}
	if service.listCalls != 0 || service.hiddenID != 0 || service.unhiddenID != 0 {
		t.Fatalf("protected service was reached: %#v", service)
	}
}

func TestApplicationHasNoLoginOrLogoutEndpoints(t *testing.T) {
	t.Parallel()

	router := protectedAdminRouter(&fakeAdminCommentService{})
	for _, path := range []string{"/api/admin/login", "/api/admin/logout"} {
		request := httptest.NewRequest(http.MethodPost, path, nil)
		request.Header.Set("Cf-Access-Jwt-Assertion", "valid-test-assertion")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Errorf("POST %s status = %d, want 404", path, response.Code)
		}
	}
}

func protectedAdminRouter(service AdminCommentService) http.Handler {
	authenticator := httpmiddleware.NewAccessAuthenticator(validAdminVerifier{}, nil)
	return NewApplicationRouter(
		&fakePinger{},
		time.Second,
		nil,
		FeatureHandlers{
			AdminAccess:   authenticator.Authenticate,
			AdminComments: NewAdminCommentHandler(service, time.Second, nil),
		},
	)
}

type validAdminVerifier struct{}

func (validAdminVerifier) Verify(context.Context, string) (adminauth.Principal, error) {
	return adminauth.Principal{ActorID: "access-subject-123"}, nil
}

type fakeAdminCommentService struct {
	listed     []comments.Comment
	status     comments.Status
	hiddenID   int64
	unhiddenID int64
	audit      comments.AuditContext
	listCalls  int
}

func (service *fakeAdminCommentService) List(
	_ context.Context,
	status comments.Status,
	_ int64,
	_ int,
) ([]comments.Comment, error) {
	service.status = status
	service.listCalls++
	return service.listed, nil
}

func (service *fakeAdminCommentService) Hide(
	_ context.Context,
	id int64,
	audit comments.AuditContext,
) (comments.Comment, error) {
	service.hiddenID = id
	service.audit = audit
	return comments.Comment{ID: id, Status: comments.StatusHidden}, nil
}

func (service *fakeAdminCommentService) Unhide(
	_ context.Context,
	id int64,
	audit comments.AuditContext,
) (comments.Comment, error) {
	service.unhiddenID = id
	service.audit = audit
	return comments.Comment{ID: id, Status: comments.StatusVisible}, nil
}
