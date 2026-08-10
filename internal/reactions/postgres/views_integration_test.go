package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	platformmigrate "github.com/home-compute-cluster/portfolio-backend/internal/platform/migrate"
	"github.com/home-compute-cluster/portfolio-backend/internal/testutil/postgrestest"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testPost = "building-a-homelab"

func TestIntegrationRecordViewDeduplicatesConcurrentVisitorAtomically(t *testing.T) {
	pool := migratedPool(t)
	store := NewViewStore(pool)
	now := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	visitor := viewHash(1)

	const requests = 30
	start := make(chan struct{})
	results := make(chan bool, requests)
	errorsFound := make(chan error, requests)
	var ready sync.WaitGroup
	ready.Add(requests)
	for range requests {
		go func() {
			ready.Done()
			<-start
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			counted, err := store.RecordView(ctx, testPost, visitor, now, 24*time.Hour)
			results <- counted
			errorsFound <- err
		}()
	}
	ready.Wait()
	close(start)

	countedRequests := 0
	for range requests {
		if err := <-errorsFound; err != nil {
			t.Fatalf("RecordView() error = %v", err)
		}
		if <-results {
			countedRequests++
		}
	}
	if countedRequests != 1 {
		t.Fatalf("counted concurrent requests = %d, want 1", countedRequests)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	count, err := store.Count(ctx, testPost)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("view count = %d, want 1", count)
	}
}

func TestIntegrationRecordViewUsesRollingWindowBoundary(t *testing.T) {
	pool := migratedPool(t)
	store := NewViewStore(pool)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Date(2026, time.August, 10, 23, 59, 0, 0, time.UTC)
	visitor := viewHash(1)
	checks := []struct {
		at   time.Time
		want bool
	}{
		{start, true},
		{start.Add(23*time.Hour + 59*time.Minute), false},
		{start.Add(24 * time.Hour), true},
	}
	for _, check := range checks {
		counted, err := store.RecordView(ctx, testPost, visitor, check.at, 24*time.Hour)
		if err != nil {
			t.Fatalf("RecordView(%s): %v", check.at, err)
		}
		if counted != check.want {
			t.Fatalf("RecordView(%s) = %t, want %t", check.at, counted, check.want)
		}
	}
	if _, err := store.RecordView(ctx, testPost, viewHash(2), start.Add(time.Minute), 24*time.Hour); err != nil {
		t.Fatalf("second visitor RecordView: %v", err)
	}
	count, err := store.Count(ctx, testPost)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("view count = %d, want 3", count)
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

func viewHash(value byte) [32]byte {
	var result [32]byte
	result[0] = value
	return result
}
