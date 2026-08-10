package comments

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCreateTrimsAndValidatesRuneLength(t *testing.T) {
	t.Parallel()

	store := &recordingStore{}
	service := NewService(store, acceptingRegistry{}, testLimits())
	comment, err := service.Create(context.Background(), CreateInput{
		PostSlug:   "known-post",
		AuthorName: "  Alice  ",
		Body:       "  你好 world  ",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if comment.AuthorName != "Alice" || comment.Body != "你好 world" {
		t.Fatalf("stored comment = %#v, want trimmed text", comment)
	}

	_, err = service.Create(context.Background(), CreateInput{
		PostSlug:   "known-post",
		AuthorName: "Alice",
		Body:       strings.Repeat("界", 11),
	})
	if !errors.Is(err, ErrInvalidBody) {
		t.Fatalf("oversized body error = %v, want ErrInvalidBody", err)
	}
}

func TestCreateRejectsWhitespaceOnlyText(t *testing.T) {
	t.Parallel()

	service := NewService(&recordingStore{}, acceptingRegistry{}, testLimits())
	tests := []CreateInput{
		{PostSlug: "known-post", AuthorName: " ", Body: "body"},
		{PostSlug: "known-post", AuthorName: "Alice", Body: "\n\t"},
	}
	for _, input := range tests {
		if _, err := service.Create(context.Background(), input); err == nil {
			t.Fatalf("Create(%#v) error = nil, want validation error", input)
		}
	}
}

func TestListVisibleValidatesPagination(t *testing.T) {
	t.Parallel()

	store := &recordingStore{}
	service := NewService(store, acceptingRegistry{}, testLimits())
	if _, err := service.ListVisible(context.Background(), "known-post", -1, 10); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("negative cursor error = %v, want ErrInvalidCursor", err)
	}
	if _, err := service.ListVisible(context.Background(), "known-post", 0, 101); !errors.Is(err, ErrInvalidCursor) {
		t.Fatalf("oversized page error = %v, want ErrInvalidCursor", err)
	}
	if _, err := service.ListVisible(context.Background(), "known-post", 0, 0); err != nil {
		t.Fatalf("default page error = %v", err)
	}
	if store.lastLimit != 25 {
		t.Fatalf("default page limit = %d, want 25", store.lastLimit)
	}
}

func testLimits() Limits {
	return Limits{
		MaxAuthorChars:     20,
		MaxCommentChars:    10,
		MaxCommentsPerPost: 100,
		DefaultPageSize:    25,
		MaximumPageSize:    100,
	}
}

type acceptingRegistry struct{}

func (acceptingRegistry) RequirePublishedPost(context.Context, string) error { return nil }

type recordingStore struct {
	lastLimit int
}

func (store *recordingStore) CreateVisibleIfUnderLimit(
	_ context.Context,
	input CreateInput,
	_ int,
) (Comment, error) {
	return Comment{AuthorName: input.AuthorName, Body: input.Body}, nil
}

func (store *recordingStore) ListVisible(
	_ context.Context,
	_ string,
	_ int64,
	limit int,
) ([]Comment, error) {
	store.lastLimit = limit
	return nil, nil
}
