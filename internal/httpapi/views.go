package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/ratelimit"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/visitor"
)

type ViewService interface {
	Record(ctx context.Context, postSlug string, visitorHash [32]byte) (bool, error)
}

type ViewHandler struct {
	service      ViewService
	identity     *visitor.Identity
	limiter      ratelimit.Limiter
	queryTimeout time.Duration
	logger       *slog.Logger
}

func NewViewHandler(
	service ViewService,
	identity *visitor.Identity,
	limiter ratelimit.Limiter,
	queryTimeout time.Duration,
	logger *slog.Logger,
) *ViewHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ViewHandler{
		service: service, identity: identity, limiter: limiter,
		queryTimeout: queryTimeout, logger: logger,
	}
}

func (handler *ViewHandler) Record(response http.ResponseWriter, request *http.Request) {
	address, ok := httpmiddleware.ClientIP(request.Context())
	if !ok {
		handler.logger.Error("view visitor identity unavailable")
		writeError(response, http.StatusInternalServerError, "internal_error")
		return
	}
	visitorHash := handler.identity.Hash(address, request.UserAgent())
	if handler.limiter != nil && !handler.limiter.Allow(visitorHash, time.Now()) {
		writeError(response, http.StatusTooManyRequests, "rate_limited")
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), handler.queryTimeout)
	defer cancel()
	if _, err := handler.service.Record(ctx, chi.URLParam(request, "slug"), visitorHash); err != nil {
		switch {
		case errors.Is(err, content.ErrInvalidSlug):
			writeError(response, http.StatusBadRequest, "invalid_request")
		case errors.Is(err, content.ErrNotFound):
			writeError(response, http.StatusNotFound, "post_not_found")
		default:
			handler.logger.Error("record view failed", "error", err, "request_id", request.Header.Get("X-Request-ID"))
			writeError(response, http.StatusInternalServerError, "internal_error")
		}
		return
	}
	response.WriteHeader(http.StatusNoContent)
}
