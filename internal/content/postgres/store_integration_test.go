package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	platformmigrate "github.com/home-compute-cluster/portfolio-backend/internal/platform/migrate"
	"github.com/home-compute-cluster/portfolio-backend/internal/testutil/postgrestest"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationPublishedPostExistsHonorsKindAndPublicationState(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `
		INSERT INTO content_items (slug, kind, status) VALUES
			('draft-post', 'post', 'draft'),
			('archived-post', 'post', 'archived'),
			('published-project', 'project', 'published')
	`); err != nil {
		t.Fatalf("seed content states: %v", err)
	}

	store := NewStore(pool)
	tests := map[string]bool{
		"building-a-homelab": true,
		"missing-post":       false,
		"draft-post":         false,
		"archived-post":      false,
		"published-project":  false,
	}
	for slug, want := range tests {
		got, err := store.PublishedPostExists(ctx, slug)
		if err != nil {
			t.Fatalf("PublishedPostExists(%q): %v", slug, err)
		}
		if got != want {
			t.Fatalf("PublishedPostExists(%q) = %t, want %t", slug, got, want)
		}
	}
}

func TestIntegrationContentRegistryConstraintsRejectInvalidRows(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	invalidRows := []struct {
		slug, kind, status string
	}{
		{"Invalid Slug", "post", "published"},
		{"valid-slug", "article", "published"},
		{"valid-slug", "post", "deleted"},
	}
	for _, row := range invalidRows {
		if _, err := pool.Exec(ctx,
			`INSERT INTO content_items (slug, kind, status) VALUES ($1, $2, $3)`,
			row.slug, row.kind, row.status,
		); err == nil {
			t.Fatalf("invalid row (%q, %q, %q) was accepted", row.slug, row.kind, row.status)
		}
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
