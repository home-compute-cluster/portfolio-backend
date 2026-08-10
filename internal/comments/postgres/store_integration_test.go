package postgres

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	platformmigrate "github.com/home-compute-cluster/portfolio-backend/internal/platform/migrate"
	"github.com/home-compute-cluster/portfolio-backend/internal/testutil/postgrestest"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testPost = "building-a-homelab"

func TestIntegrationCreateVisibleIfUnderLimitIsAtomic(t *testing.T) {
	pool := migratedPool(t)
	store := NewStore(pool)

	const (
		requests = 12
		maximum  = 3
	)
	start := make(chan struct{})
	errorsFound := make(chan error, requests)
	var ready sync.WaitGroup
	ready.Add(requests)
	for index := range requests {
		go func() {
			ready.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_, err := store.CreateVisibleIfUnderLimit(ctx, comments.CreateInput{
				PostSlug:    testPost,
				AuthorName:  "visitor",
				Body:        "comment",
				VisitorHash: hash(byte(index + 1)),
			}, maximum)
			errorsFound <- err
		}()
	}
	ready.Wait()
	close(start)

	created := 0
	full := 0
	for range requests {
		err := <-errorsFound
		switch {
		case err == nil:
			created++
		case errors.Is(err, comments.ErrPostFull):
			full++
		default:
			t.Fatalf("concurrent creation error = %v", err)
		}
	}
	if created != maximum || full != requests-maximum {
		t.Fatalf("concurrent results = %d created, %d full; want %d and %d", created, full, maximum, requests-maximum)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var count int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM comments WHERE post_slug = $1`, testPost).Scan(&count); err != nil {
		t.Fatalf("count stored comments: %v", err)
	}
	if count != maximum {
		t.Fatalf("stored comment count = %d, want %d", count, maximum)
	}
}

func TestIntegrationListVisibleUsesStableDescendingCursor(t *testing.T) {
	pool := migratedPool(t)
	store := NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	created := make([]comments.Comment, 0, 3)
	for index := range 3 {
		comment, err := store.CreateVisibleIfUnderLimit(ctx, comments.CreateInput{
			PostSlug:    testPost,
			AuthorName:  "visitor",
			Body:        "comment",
			VisitorHash: hash(byte(index + 1)),
		}, 10)
		if err != nil {
			t.Fatalf("create comment %d: %v", index, err)
		}
		created = append(created, comment)
	}

	firstPage, err := store.ListVisible(ctx, testPost, 0, 2)
	if err != nil {
		t.Fatalf("first ListVisible: %v", err)
	}
	if len(firstPage) != 2 || firstPage[0].ID != created[2].ID || firstPage[1].ID != created[1].ID {
		t.Fatalf("first page IDs = %v, want newest two", commentIDs(firstPage))
	}

	secondPage, err := store.ListVisible(ctx, testPost, firstPage[1].ID, 2)
	if err != nil {
		t.Fatalf("second ListVisible: %v", err)
	}
	if len(secondPage) != 1 || secondPage[0].ID != created[0].ID {
		t.Fatalf("second page IDs = %v, want oldest", commentIDs(secondPage))
	}
}

func TestIntegrationCommentConstraintsRejectInvalidPersistentState(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	invalid := []struct {
		author, body, status string
		hiddenAt             any
		hash                 []byte
	}{
		{"", "body", "visible", nil, make([]byte, 32)},
		{" author ", "body", "visible", nil, make([]byte, 32)},
		{"author", "", "visible", nil, make([]byte, 32)},
		{"author", "body", "deleted", nil, make([]byte, 32)},
		{"author", "body", "hidden", nil, make([]byte, 32)},
		{"author", "body", "visible", time.Now(), make([]byte, 32)},
		{"author", "body", "visible", nil, make([]byte, 31)},
	}
	for _, row := range invalid {
		if _, err := pool.Exec(ctx, `
			INSERT INTO comments (post_slug, author_name, body, status, hidden_at, visitor_hash)
			VALUES ($1, $2, $3, $4, $5, $6)
		`, testPost, row.author, row.body, row.status, row.hiddenAt, row.hash); err == nil {
			t.Fatalf("invalid comment (%q, %q, %q) was accepted", row.author, row.body, row.status)
		}
	}
}

func TestIntegrationModerationPreservesVisibleCommentCap(t *testing.T) {
	pool := migratedPool(t)
	store := NewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	first, err := store.CreateVisibleIfUnderLimit(ctx, comments.CreateInput{
		PostSlug: testPost, AuthorName: "first", Body: "comment", VisitorHash: hash(1),
	}, 1)
	if err != nil {
		t.Fatalf("create first comment: %v", err)
	}
	hidden, err := store.SetVisibility(ctx, first.ID, comments.StatusHidden, 1)
	if err != nil {
		t.Fatalf("hide first comment: %v", err)
	}
	if hidden.Status != comments.StatusHidden || hidden.HiddenAt == nil {
		t.Fatalf("hidden state = %#v", hidden)
	}

	second, err := store.CreateVisibleIfUnderLimit(ctx, comments.CreateInput{
		PostSlug: testPost, AuthorName: "second", Body: "comment", VisitorHash: hash(2),
	}, 1)
	if err != nil {
		t.Fatalf("create replacement visible comment: %v", err)
	}
	if _, err := store.SetVisibility(ctx, first.ID, comments.StatusVisible, 1); !errors.Is(err, comments.ErrPostFull) {
		t.Fatalf("unhide over cap error = %v, want ErrPostFull", err)
	}
	if _, err := store.SetVisibility(ctx, second.ID, comments.StatusHidden, 1); err != nil {
		t.Fatalf("hide second comment: %v", err)
	}
	unhidden, err := store.SetVisibility(ctx, first.ID, comments.StatusVisible, 1)
	if err != nil {
		t.Fatalf("unhide first comment: %v", err)
	}
	if unhidden.Status != comments.StatusVisible || unhidden.HiddenAt != nil {
		t.Fatalf("unhidden state = %#v", unhidden)
	}
	if _, err := store.SetVisibility(ctx, first.ID, comments.StatusVisible, 1); err != nil {
		t.Fatalf("idempotent unhide: %v", err)
	}

	hiddenComments, err := store.ListForModeration(ctx, comments.StatusHidden, 0, 10)
	if err != nil {
		t.Fatalf("list hidden comments: %v", err)
	}
	if len(hiddenComments) != 1 || hiddenComments[0].ID != second.ID {
		t.Fatalf("hidden comment IDs = %v, want [%d]", commentIDs(hiddenComments), second.ID)
	}
}

func migratedPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	pool := postgrestest.Open(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	migrationFS := os.DirFS(filepath.Join("..", "..", "..", "migrations"))
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := platformmigrate.Up(ctx, pool, migrationFS, logger); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}
	return pool
}

func hash(value byte) [32]byte {
	var result [32]byte
	result[0] = value
	return result
}

func commentIDs(values []comments.Comment) []int64 {
	result := make([]int64, len(values))
	for index, comment := range values {
		result[index] = comment.ID
	}
	return result
}
