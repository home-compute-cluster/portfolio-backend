package postgres

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	platformmigrate "github.com/home-compute-cluster/portfolio-backend/internal/platform/migrate"
	"github.com/home-compute-cluster/portfolio-backend/internal/testutil/postgrestest"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestIntegrationContentStateHonorsRegistryAndPublicationState(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `
		INSERT INTO content_items (slug, kind, status) VALUES
			('draft-post', 'post', 'draft'),
			('archived-post', 'post', 'archived'),
			('published-future-kind', 'essay', 'published')
	`); err != nil {
		t.Fatalf("seed content states: %v", err)
	}

	store := NewStore(pool)
	tests := map[string]content.State{
		"building-a-homelab":    {Published: true, CommentsEnabled: true},
		"k3s-cluster":           {Published: true, CommentsEnabled: true},
		"lan-drop":              {Published: true, CommentsEnabled: true},
		"obsync":                {Published: true, CommentsEnabled: true},
		"relic-overlay":         {Published: true, CommentsEnabled: true},
		"cs2105":                {Published: true, CommentsEnabled: true},
		"i7-9700k":              {Published: true, CommentsEnabled: true},
		"warframe":              {Published: true, CommentsEnabled: true},
		"published-future-kind": {Published: true, CommentsEnabled: true},
		"missing-content":       {},
		"draft-post":            {CommentsEnabled: true},
		"archived-post":         {CommentsEnabled: true},
	}
	for slug, want := range tests {
		got, err := store.ContentState(ctx, slug)
		if err != nil {
			t.Fatalf("ContentState(%q): %v", slug, err)
		}
		if got != want {
			t.Fatalf("ContentState(%q) = %#v, want %#v", slug, got, want)
		}
	}
}

func TestIntegrationSyncSnapshotUpsertsArchivesAndIsIdempotent(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	store := NewStore(pool)
	snapshot := content.Snapshot{
		SchemaVersion: content.ManifestSchemaVersion,
		Source:        "portfolio-site",
		Revision:      "0123456789abcdef0123456789abcdef01234567",
		Items: []content.Item{
			{Slug: "building-a-homelab", Kind: "blog", Status: content.StatusPublished, CommentsEnabled: false},
			{Slug: "new-reading", Kind: "essay", Status: content.StatusPublished, CommentsEnabled: true},
		},
	}

	result, err := store.SyncSnapshot(ctx, snapshot)
	if err != nil {
		t.Fatalf("SyncSnapshot() error = %v", err)
	}
	if result.Changed != 2 || result.Archived == 0 {
		t.Fatalf("SyncSnapshot() result = %#v, want 2 changed and at least 1 archived", result)
	}

	state, err := store.ContentState(ctx, "building-a-homelab")
	if err != nil {
		t.Fatalf("read synchronized state: %v", err)
	}
	if state != (content.State{Published: true, CommentsEnabled: false}) {
		t.Fatalf("synchronized state = %#v", state)
	}
	state, err = store.ContentState(ctx, "k3s-cluster")
	if err != nil {
		t.Fatalf("read archived state: %v", err)
	}
	if state.Published {
		t.Fatalf("absent content remained published: %#v", state)
	}

	result, err = store.SyncSnapshot(ctx, snapshot)
	if err != nil {
		t.Fatalf("second SyncSnapshot() error = %v", err)
	}
	if result != (content.SyncResult{}) {
		t.Fatalf("second SyncSnapshot() result = %#v, want no changes", result)
	}
}

func TestIntegrationSyncSnapshotRejectsAnotherSourcesSlugAndRollsBack(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := pool.Exec(ctx, `
		INSERT INTO content_items (slug, kind, status, comments_enabled, managed_by)
		VALUES ('external-content', 'note', 'published', true, 'other-source')
	`); err != nil {
		t.Fatalf("seed external content: %v", err)
	}

	store := NewStore(pool)
	_, err := store.SyncSnapshot(ctx, content.Snapshot{
		SchemaVersion: content.ManifestSchemaVersion,
		Source:        "portfolio-site",
		Revision:      "0123456789abcdef0123456789abcdef01234567",
		Items: []content.Item{
			{Slug: "building-a-homelab", Kind: "post", Status: content.StatusPublished, CommentsEnabled: false},
			{Slug: "external-content", Kind: "note", Status: content.StatusPublished, CommentsEnabled: true},
		},
	})
	if err == nil {
		t.Fatal("SyncSnapshot() error = nil, want ownership conflict")
	}

	state, stateErr := store.ContentState(ctx, "building-a-homelab")
	if stateErr != nil {
		t.Fatalf("read state after rollback: %v", stateErr)
	}
	if !state.CommentsEnabled {
		t.Fatal("ownership conflict did not roll back earlier updates")
	}
}

func TestIntegrationContentRegistryConstraintsRejectInvalidRows(t *testing.T) {
	pool := migratedPool(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	invalidRows := []struct {
		slug, kind, status, managedBy string
	}{
		{"Invalid Slug", "post", "published", "portfolio-site"},
		{"valid-slug", "Invalid Kind", "published", "portfolio-site"},
		{"valid-slug", "", "published", "portfolio-site"},
		{"valid-slug", "post", "deleted", "portfolio-site"},
		{"valid-slug", "post", "published", "Invalid Source"},
	}
	for _, row := range invalidRows {
		if _, err := pool.Exec(ctx,
			`INSERT INTO content_items (slug, kind, status, managed_by) VALUES ($1, $2, $3, $4)`,
			row.slug, row.kind, row.status, row.managedBy,
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
