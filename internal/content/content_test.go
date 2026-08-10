package content

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestValidSlug(t *testing.T) {
	t.Parallel()

	tests := map[string]bool{
		"post":                   true,
		"building-a-homelab":     true,
		"post-2":                 true,
		"":                       false,
		"UPPERCASE":              false,
		"two--hyphens":           false,
		"-leading":               false,
		"trailing-":              false,
		"has spaces":             false,
		"unicode-文章":             false,
		strings.Repeat("a", 101): false,
	}

	for slug, want := range tests {
		t.Run(slug, func(t *testing.T) {
			t.Parallel()
			if got := ValidSlug(slug); got != want {
				t.Fatalf("ValidSlug(%q) = %t, want %t", slug, got, want)
			}
		})
	}
}

func TestRequirePublishedPost(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{exists: true})
	if err := service.RequirePublishedPost(context.Background(), "known-post"); err != nil {
		t.Fatalf("RequirePublishedPost() error = %v", err)
	}

	service = NewService(stubStore{})
	if err := service.RequirePublishedPost(context.Background(), "unknown-post"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown post error = %v, want ErrNotFound", err)
	}

	if err := service.RequirePublishedPost(context.Background(), "Not Valid"); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("invalid slug error = %v, want ErrInvalidSlug", err)
	}
}

type stubStore struct {
	exists bool
	err    error
}

func (store stubStore) PublishedPostExists(context.Context, string) (bool, error) {
	return store.exists, store.err
}
