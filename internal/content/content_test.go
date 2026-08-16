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

func TestRequirePublishedContent(t *testing.T) {
	t.Parallel()

	service := NewService(stubStore{state: State{Published: true}})
	if err := service.RequirePublishedContent(context.Background(), "known-content"); err != nil {
		t.Fatalf("RequirePublishedContent() error = %v", err)
	}

	service = NewService(stubStore{})
	if err := service.RequirePublishedContent(context.Background(), "unknown-content"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown content error = %v, want ErrNotFound", err)
	}

	if err := service.RequirePublishedContent(context.Background(), "Not Valid"); !errors.Is(err, ErrInvalidSlug) {
		t.Fatalf("invalid slug error = %v, want ErrInvalidSlug", err)
	}
}

type stubStore struct {
	state State
	err   error
}

func (store stubStore) ContentState(context.Context, string) (State, error) {
	return store.state, store.err
}

func TestRequireCommentsEnabled(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		state State
		want  error
	}{
		{name: "enabled", state: State{Published: true, CommentsEnabled: true}},
		{name: "disabled", state: State{Published: true}, want: ErrCommentsDisabled},
		{name: "unpublished", state: State{CommentsEnabled: true}, want: ErrNotFound},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := NewService(stubStore{state: test.state})
			err := service.RequireCommentsEnabled(context.Background(), "known-content")
			if !errors.Is(err, test.want) {
				t.Fatalf("RequireCommentsEnabled() error = %v, want %v", err, test.want)
			}
		})
	}
}
