package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/visitor"
)

func TestRecordViewReturns204AndPassesVisitorHash(t *testing.T) {
	t.Parallel()

	service := &fakeViewService{counted: true}
	response := viewRequest(t, service, "/api/posts/known-post/view")
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204; body = %s", response.Code, response.Body.String())
	}
	if service.slug != "known-post" || service.visitorHash == ([32]byte{}) {
		t.Fatalf("view input = slug %q, hash %x", service.slug, service.visitorHash)
	}
}

func TestRecordViewReturns204WhenDeduplicated(t *testing.T) {
	t.Parallel()

	response := viewRequest(t, &fakeViewService{counted: false}, "/api/posts/known-post/view")
	if response.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", response.Code)
	}
}

func TestRecordViewMapsErrorsWithoutLeaking(t *testing.T) {
	t.Parallel()

	tests := []struct {
		err  error
		want int
	}{
		{content.ErrInvalidSlug, http.StatusBadRequest},
		{content.ErrNotFound, http.StatusNotFound},
		{errors.New("private database detail"), http.StatusInternalServerError},
	}
	for _, test := range tests {
		response := viewRequest(t, &fakeViewService{err: test.err}, "/api/posts/known-post/view")
		if response.Code != test.want {
			t.Fatalf("error %v status = %d, want %d", test.err, response.Code, test.want)
		}
		if strings.Contains(response.Body.String(), "private database detail") {
			t.Fatal("view response leaked internal error")
		}
	}
}

func viewRequest(t *testing.T, service *fakeViewService, target string) *httptest.ResponseRecorder {
	t.Helper()
	handler := NewViewHandler(
		service,
		visitor.NewIdentity([]byte("0123456789abcdef0123456789abcdef")),
		nil,
		time.Second,
		nil,
	)
	router := NewApplicationRouter(
		&fakePinger{}, time.Second, nil,
		FeatureHandlers{ClientIP: httpmiddleware.NewClientIPResolver(nil), Views: handler},
	)
	request := httptest.NewRequest(http.MethodPost, target, nil)
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)
	return response
}

type fakeViewService struct {
	counted     bool
	err         error
	slug        string
	visitorHash [32]byte
}

func (service *fakeViewService) Record(
	_ context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	service.slug = postSlug
	service.visitorHash = visitorHash
	return service.counted, service.err
}
