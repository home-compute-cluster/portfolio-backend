package content

import (
	"context"
	"errors"
	"strings"
)

const MaxSlugChars = 100

var (
	ErrInvalidSlug      = errors.New("invalid content slug")
	ErrNotFound         = errors.New("content not found")
	ErrCommentsDisabled = errors.New("comments disabled")
)

// State contains the registry policy needed by dynamic content features.
type State struct {
	Published       bool
	CommentsEnabled bool
}

type Store interface {
	ContentState(ctx context.Context, slug string) (State, error)
}

type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// RequirePublishedContent verifies that slug identifies a registered,
// published content item that may use backend-owned dynamic features.
func (service *Service) RequirePublishedContent(ctx context.Context, slug string) error {
	if !ValidSlug(slug) {
		return ErrInvalidSlug
	}

	state, err := service.store.ContentState(ctx, slug)
	if err != nil {
		return err
	}
	if !state.Published {
		return ErrNotFound
	}

	return nil
}

// RequireCommentsEnabled verifies that slug is published and its source
// content explicitly enables the public comment feature.
func (service *Service) RequireCommentsEnabled(ctx context.Context, slug string) error {
	if !ValidSlug(slug) {
		return ErrInvalidSlug
	}

	state, err := service.store.ContentState(ctx, slug)
	if err != nil {
		return err
	}
	if !state.Published {
		return ErrNotFound
	}
	if !state.CommentsEnabled {
		return ErrCommentsDisabled
	}

	return nil
}

func ValidSlug(slug string) bool {
	if slug == "" || len(slug) > MaxSlugChars || strings.TrimSpace(slug) != slug {
		return false
	}

	previousHyphen := false
	for index, character := range slug {
		isLetter := character >= 'a' && character <= 'z'
		isDigit := character >= '0' && character <= '9'
		if isLetter || isDigit {
			previousHyphen = false
			continue
		}
		if character != '-' || index == 0 || previousHyphen {
			return false
		}
		previousHyphen = true
	}

	return !previousHyphen
}
