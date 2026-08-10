package comments

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidAuthor = errors.New("invalid comment author")
	ErrInvalidBody   = errors.New("invalid comment body")
	ErrInvalidCursor = errors.New("invalid comment cursor")
	ErrPostFull      = errors.New("post comment limit reached")
	ErrNotFound      = errors.New("comment not found")
)

type Status string

const (
	StatusVisible Status = "visible"
	StatusHidden  Status = "hidden"
)

type Comment struct {
	ID         int64
	PostSlug   string
	AuthorName string
	Body       string
	Status     Status
	CreatedAt  time.Time
	HiddenAt   *time.Time
}

type CreateInput struct {
	PostSlug    string
	AuthorName  string
	Body        string
	VisitorHash [32]byte
}

type Store interface {
	CreateVisibleIfUnderLimit(ctx context.Context, input CreateInput, maxVisible int) (Comment, error)
	ListVisible(ctx context.Context, postSlug string, beforeID int64, limit int) ([]Comment, error)
}

type ContentRegistry interface {
	RequirePublishedPost(ctx context.Context, slug string) error
}

type Limits struct {
	MaxAuthorChars     int
	MaxCommentChars    int
	MaxCommentsPerPost int
	DefaultPageSize    int
	MaximumPageSize    int
}

type Service struct {
	store   Store
	content ContentRegistry
	limits  Limits
}

func NewService(store Store, content ContentRegistry, limits Limits) *Service {
	return &Service{store: store, content: content, limits: limits}
}

func (service *Service) Create(ctx context.Context, input CreateInput) (Comment, error) {
	if err := service.content.RequirePublishedPost(ctx, input.PostSlug); err != nil {
		return Comment{}, err
	}

	input.AuthorName = strings.TrimSpace(input.AuthorName)
	if !validText(input.AuthorName, service.limits.MaxAuthorChars) {
		return Comment{}, ErrInvalidAuthor
	}

	input.Body = strings.TrimSpace(input.Body)
	if !validText(input.Body, service.limits.MaxCommentChars) {
		return Comment{}, ErrInvalidBody
	}

	return service.store.CreateVisibleIfUnderLimit(
		ctx,
		input,
		service.limits.MaxCommentsPerPost,
	)
}

func (service *Service) ListVisible(
	ctx context.Context,
	postSlug string,
	beforeID int64,
	limit int,
) ([]Comment, error) {
	if beforeID < 0 || limit < 0 || limit > service.limits.MaximumPageSize {
		return nil, ErrInvalidCursor
	}
	if err := service.content.RequirePublishedPost(ctx, postSlug); err != nil {
		return nil, err
	}
	if limit == 0 {
		limit = service.limits.DefaultPageSize
	}

	return service.store.ListVisible(ctx, postSlug, beforeID, limit)
}

func validText(value string, maximum int) bool {
	return value != "" && utf8.ValidString(value) && utf8.RuneCountInString(value) <= maximum
}
