package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
)

// AdminCommentHandler exposes comment moderation only through the Access-protected admin route group.
type AdminCommentHandler struct {
	service      AdminCommentService
	queryTimeout time.Duration
	logger       *slog.Logger
}

type AdminCommentService interface {
	List(ctx context.Context, status comments.Status, beforeID int64, limit int) ([]comments.Comment, error)
	Hide(ctx context.Context, id int64, audit comments.AuditContext) (comments.Comment, error)
	Unhide(ctx context.Context, id int64, audit comments.AuditContext) (comments.Comment, error)
}

func NewAdminCommentHandler(
	service AdminCommentService,
	queryTimeout time.Duration,
	logger *slog.Logger,
) *AdminCommentHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &AdminCommentHandler{service: service, queryTimeout: queryTimeout, logger: logger}
}

func (handler *AdminCommentHandler) List(response http.ResponseWriter, request *http.Request) {
	if _, ok := httpmiddleware.AdminPrincipal(request.Context()); !ok {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}
	beforeID, err := optionalPositiveInt64(request.URL.Query().Get("before_id"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_cursor")
		return
	}
	limit, err := optionalPositiveInt(request.URL.Query().Get("limit"))
	if err != nil {
		writeError(response, http.StatusBadRequest, "invalid_cursor")
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), handler.queryTimeout)
	defer cancel()
	values, err := handler.service.List(ctx, comments.Status(request.URL.Query().Get("status")), beforeID, limit)
	if err != nil {
		handler.handleError(response, request, err)
		return
	}

	result := struct {
		Comments     []moderationCommentResponse `json:"comments"`
		NextBeforeID *int64                      `json:"next_before_id"`
	}{Comments: make([]moderationCommentResponse, len(values))}
	for index, comment := range values {
		result.Comments[index] = moderationComment(comment)
	}
	if len(values) > 0 && (limit == 0 || len(values) == limit) {
		next := values[len(values)-1].ID
		result.NextBeforeID = &next
	}
	writeJSON(response, http.StatusOK, result)
}

func (handler *AdminCommentHandler) Hide(response http.ResponseWriter, request *http.Request) {
	handler.setVisibility(response, request, comments.StatusHidden, handler.service.Hide)
}

func (handler *AdminCommentHandler) Unhide(response http.ResponseWriter, request *http.Request) {
	handler.setVisibility(response, request, comments.StatusVisible, handler.service.Unhide)
}

func (handler *AdminCommentHandler) setVisibility(
	response http.ResponseWriter,
	request *http.Request,
	desired comments.Status,
	operation func(context.Context, int64, comments.AuditContext) (comments.Comment, error),
) {
	id, err := strconv.ParseInt(chi.URLParam(request, "id"), 10, 64)
	if err != nil || id <= 0 {
		writeError(response, http.StatusNotFound, "comment_not_found")
		return
	}
	principal, ok := httpmiddleware.AdminPrincipal(request.Context())
	if !ok {
		writeError(response, http.StatusUnauthorized, "unauthorized")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), handler.queryTimeout)
	defer cancel()
	comment, err := operation(ctx, id, comments.AuditContext{
		ActorID:   principal.ActorID,
		RequestID: chimiddleware.GetReqID(request.Context()),
	})
	if err != nil {
		handler.handleError(response, request, err)
		return
	}
	handler.logger.Info(
		"comment moderation completed",
		"error_category", "moderation_action",
		"request_id", chimiddleware.GetReqID(request.Context()),
		"actor_id", principal.ActorID,
		"resource_type", "comment",
		"resource_id", id,
		"desired_state", desired,
	)
	writeJSON(response, http.StatusOK, moderationComment(comment))
}

func (handler *AdminCommentHandler) handleError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, comments.ErrInvalidCursor), errors.Is(err, comments.ErrInvalidStatus):
		writeError(response, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, comments.ErrNotFound):
		writeError(response, http.StatusNotFound, "comment_not_found")
	case errors.Is(err, comments.ErrPostFull):
		writeError(response, http.StatusConflict, "comment_limit_reached")
	case errors.Is(err, comments.ErrInvalidAuditActor):
		writeError(response, http.StatusUnauthorized, "unauthorized")
	default:
		handler.logger.Error("comment moderation failed", "error", err, "request_id", chimiddleware.GetReqID(request.Context()))
		writeError(response, http.StatusInternalServerError, "internal_error")
	}
}

type moderationCommentResponse struct {
	ID         int64           `json:"id"`
	PostSlug   string          `json:"post_slug"`
	AuthorName string          `json:"author_name"`
	Body       string          `json:"body"`
	Status     comments.Status `json:"status"`
	CreatedAt  time.Time       `json:"created_at"`
	HiddenAt   *time.Time      `json:"hidden_at"`
}

func moderationComment(comment comments.Comment) moderationCommentResponse {
	return moderationCommentResponse{
		ID:         comment.ID,
		PostSlug:   comment.PostSlug,
		AuthorName: comment.AuthorName,
		Body:       comment.Body,
		Status:     comment.Status,
		CreatedAt:  comment.CreatedAt,
		HiddenAt:   comment.HiddenAt,
	}
}
