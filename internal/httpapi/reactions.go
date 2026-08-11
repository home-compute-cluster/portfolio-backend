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
	"github.com/home-compute-cluster/portfolio-backend/internal/reactions"
)

// LikeApplication exposes explicit desired-state like operations to HTTP.
type LikeApplication interface {
	Like(ctx context.Context, postSlug string, visitorHash [32]byte) (bool, error)
	Unlike(ctx context.Context, postSlug string, visitorHash [32]byte) (bool, error)
}

// StatsApplication exposes public post totals to HTTP.
type StatsApplication interface {
	Get(ctx context.Context, postSlug string) (reactions.Stats, error)
}

// ReactionHandler serves public like, unlike, and aggregate-statistics requests.
type ReactionHandler struct {
	likes        LikeApplication
	stats        StatsApplication
	identity     *visitor.Identity
	likeLimiter  ratelimit.Limiter
	queryTimeout time.Duration
	logger       *slog.Logger
}

// NewReactionHandler constructs the HTTP boundary for Iteration 8 reactions.
func NewReactionHandler(
	likes LikeApplication,
	stats StatsApplication,
	identity *visitor.Identity,
	likeLimiter ratelimit.Limiter,
	queryTimeout time.Duration,
	logger *slog.Logger,
) *ReactionHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &ReactionHandler{
		likes: likes, stats: stats, identity: identity, likeLimiter: likeLimiter,
		queryTimeout: queryTimeout, logger: logger,
	}
}

// Like ensures the current pseudonymous visitor likes the requested post.
func (handler *ReactionHandler) Like(response http.ResponseWriter, request *http.Request) {
	handler.setLikeState(response, request, handler.likes.Like)
}

// Unlike ensures the current pseudonymous visitor no longer likes the requested post.
func (handler *ReactionHandler) Unlike(response http.ResponseWriter, request *http.Request) {
	handler.setLikeState(response, request, handler.likes.Unlike)
}

// Stats returns public view and like totals without exposing visitor identities.
func (handler *ReactionHandler) Stats(response http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), handler.queryTimeout)
	defer cancel()
	stats, err := handler.stats.Get(ctx, chi.URLParam(request, "slug"))
	if err != nil {
		handler.handleError(response, request, err)
		return
	}
	writeJSON(response, http.StatusOK, struct {
		Views int64 `json:"views"`
		Likes int64 `json:"likes"`
	}{Views: stats.Views, Likes: stats.Likes})
}

// setLikeState resolves the visitor identity and executes one desired-state operation.
func (handler *ReactionHandler) setLikeState(
	response http.ResponseWriter,
	request *http.Request,
	operation func(context.Context, string, [32]byte) (bool, error),
) {
	address, ok := httpmiddleware.ClientIP(request.Context())
	if !ok {
		handler.logger.Error("like visitor identity unavailable")
		writeError(response, http.StatusInternalServerError, "internal_error")
		return
	}
	visitorHash := handler.identity.Hash(address, request.UserAgent())
	if handler.likeLimiter != nil && !handler.likeLimiter.Allow(visitorHash, time.Now()) {
		writeError(response, http.StatusTooManyRequests, "rate_limited")
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), handler.queryTimeout)
	defer cancel()
	if _, err := operation(ctx, chi.URLParam(request, "slug"), visitorHash); err != nil {
		handler.handleError(response, request, err)
		return
	}
	response.WriteHeader(http.StatusNoContent)
}

// handleError maps application errors to stable public responses and logs unexpected failures.
func (handler *ReactionHandler) handleError(
	response http.ResponseWriter,
	request *http.Request,
	err error,
) {
	switch {
	case errors.Is(err, content.ErrInvalidSlug):
		writeError(response, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, content.ErrNotFound):
		writeError(response, http.StatusNotFound, "post_not_found")
	default:
		handler.logger.Error("reaction request failed", "error", err, "request_id", request.Header.Get("X-Request-ID"))
		writeError(response, http.StatusInternalServerError, "internal_error")
	}
}
