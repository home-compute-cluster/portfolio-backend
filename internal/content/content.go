package content

import (
	"context"
	"errors"
	"strings"
)

const MaxSlugChars = 100

var (
	ErrInvalidSlug = errors.New("invalid content slug")
	ErrNotFound    = errors.New("content not found")
)

type Store interface {
	PublishedContentExists(ctx context.Context, slug string) (bool, error)
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

	exists, err := service.store.PublishedContentExists(ctx, slug)
	if err != nil {
		return err
	}
	if !exists {
		return ErrNotFound
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
