package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/ratelimit"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/visitor"
)

const maximumCommentRequestBytes int64 = 16 * 1024

type CommentService interface {
	Create(ctx context.Context, input comments.CreateInput) (comments.Comment, error)
	ListVisible(ctx context.Context, postSlug string, beforeID int64, limit int) ([]comments.Comment, error)
}

type CommentHandler struct {
	service      CommentService
	identity     *visitor.Identity
	limiter      ratelimit.Limiter
	queryTimeout time.Duration
	logger       *slog.Logger
}

func NewCommentHandler(
	service CommentService,
	identity *visitor.Identity,
	limiter ratelimit.Limiter,
	queryTimeout time.Duration,
	logger *slog.Logger,
) *CommentHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &CommentHandler{
		service:      service,
		identity:     identity,
		limiter:      limiter,
		queryTimeout: queryTimeout,
		logger:       logger,
	}
}

func (handler *CommentHandler) List(response http.ResponseWriter, request *http.Request) {
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
	values, err := handler.service.ListVisible(ctx, chi.URLParam(request, "slug"), beforeID, limit)
	if err != nil {
		handler.handleError(response, request, err)
		return
	}

	result := commentListResponse{Comments: make([]commentResponse, len(values))}
	for index, comment := range values {
		result.Comments[index] = publicComment(comment)
	}
	if len(values) > 0 && (limit == 0 || len(values) == limit) {
		next := values[len(values)-1].ID
		result.NextBeforeID = &next
	}
	writeJSON(response, http.StatusOK, result)
}

func (handler *CommentHandler) Create(response http.ResponseWriter, request *http.Request) {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(response, http.StatusUnsupportedMediaType, "unsupported_media_type")
		return
	}

	request.Body = http.MaxBytesReader(response, request.Body, maximumCommentRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	var payload struct {
		AuthorName string `json:"author_name"`
		Body       string `json:"body"`
		Website    string `json:"website"`
	}
	if err := decoder.Decode(&payload); err != nil {
		handleDecodeError(response, err)
		return
	}
	if err := ensureJSONEnd(decoder); err != nil {
		handleDecodeError(response, err)
		return
	}

	// Bots that populate the hidden website field receive an indistinguishable
	// success response, but nothing is persisted.
	if payload.Website != "" {
		response.WriteHeader(http.StatusNoContent)
		return
	}

	address, ok := httpmiddleware.ClientIP(request.Context())
	if !ok {
		handler.logger.Error("comment visitor identity unavailable")
		writeError(response, http.StatusInternalServerError, "internal_error")
		return
	}
	visitorHash := handler.identity.Hash(address, request.UserAgent())
	if handler.limiter != nil && !handler.limiter.Allow(visitorHash, time.Now()) {
		handler.logger.Warn(
			"rate limit rejected",
			"error_category", "rate_limit_rejected",
			"request_id", chimiddleware.GetReqID(request.Context()),
		)
		writeError(response, http.StatusTooManyRequests, "rate_limited")
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), handler.queryTimeout)
	defer cancel()
	comment, err := handler.service.Create(ctx, comments.CreateInput{
		PostSlug:    chi.URLParam(request, "slug"),
		AuthorName:  payload.AuthorName,
		Body:        payload.Body,
		VisitorHash: visitorHash,
	})
	if err != nil {
		handler.handleError(response, request, err)
		return
	}

	writeJSON(response, http.StatusCreated, publicComment(comment))
}

func (handler *CommentHandler) handleError(response http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, content.ErrInvalidSlug),
		errors.Is(err, comments.ErrInvalidAuthor),
		errors.Is(err, comments.ErrInvalidBody),
		errors.Is(err, comments.ErrInvalidCursor):
		writeError(response, http.StatusBadRequest, "invalid_request")
	case errors.Is(err, content.ErrNotFound):
		writeError(response, http.StatusNotFound, "post_not_found")
	case errors.Is(err, comments.ErrPostFull):
		writeError(response, http.StatusConflict, "comment_limit_reached")
	default:
		handler.logger.Error("comment request failed", "error", err, "request_id", chimiddleware.GetReqID(request.Context()))
		writeError(response, http.StatusInternalServerError, "internal_error")
	}
}

type commentResponse struct {
	ID         int64     `json:"id"`
	AuthorName string    `json:"author_name"`
	Body       string    `json:"body"`
	CreatedAt  time.Time `json:"created_at"`
}

type commentListResponse struct {
	Comments     []commentResponse `json:"comments"`
	NextBeforeID *int64            `json:"next_before_id"`
}

func publicComment(comment comments.Comment) commentResponse {
	return commentResponse{
		ID:         comment.ID,
		AuthorName: comment.AuthorName,
		Body:       comment.Body,
		CreatedAt:  comment.CreatedAt,
	}
}

func optionalPositiveInt64(value string) (int64, error) {
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return 0, comments.ErrInvalidCursor
	}
	return parsed, nil
}

func optionalPositiveInt(value string) (int, error) {
	parsed, err := optionalPositiveInt64(value)
	return int(parsed), err
}

func ensureJSONEnd(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("multiple JSON values")
	}
	return err
}

func handleDecodeError(response http.ResponseWriter, err error) {
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		writeError(response, http.StatusRequestEntityTooLarge, "request_too_large")
		return
	}
	writeError(response, http.StatusBadRequest, "malformed_json")
}

func writeError(response http.ResponseWriter, status int, code string) {
	writeJSON(response, status, struct {
		Error string `json:"error"`
	}{Error: code})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json; charset=utf-8")
	response.Header().Set("Cache-Control", "no-store")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
