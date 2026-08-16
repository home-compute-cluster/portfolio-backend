package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const contentSyncLockKey int64 = 0x636f6e74656e7473

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// ContentState returns the dynamic-feature policy for one registered item.
// Missing and unpublished rows both report Published=false to callers.
func (store *Store) ContentState(ctx context.Context, slug string) (content.State, error) {
	var state content.State
	err := store.pool.QueryRow(ctx, `
		SELECT status = 'published', comments_enabled
		FROM content_items
		WHERE slug = $1
	`, slug).Scan(&state.Published, &state.CommentsEnabled)
	if errors.Is(err, pgx.ErrNoRows) {
		return content.State{}, nil
	}
	if err != nil {
		return content.State{}, fmt.Errorf("read content state: %w", err)
	}

	return state, nil
}

// SyncSnapshot upserts one source's complete manifest and archives identities
// that disappeared. The advisory lock and transaction make concurrent jobs
// serialize, while managed_by prevents one source from taking another's slug.
func (store *Store) SyncSnapshot(ctx context.Context, snapshot content.Snapshot) (content.SyncResult, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return content.SyncResult{}, fmt.Errorf("begin content sync: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, contentSyncLockKey); err != nil {
		return content.SyncResult{}, fmt.Errorf("lock content sync: %w", err)
	}

	result := content.SyncResult{}
	slugs := make([]string, len(snapshot.Items))
	for index, item := range snapshot.Items {
		slugs[index] = item.Slug

		var owner string
		err := tx.QueryRow(ctx, `SELECT managed_by FROM content_items WHERE slug = $1`, item.Slug).Scan(&owner)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return content.SyncResult{}, fmt.Errorf("read content owner: %w", err)
		}
		if err == nil && owner != snapshot.Source {
			return content.SyncResult{}, fmt.Errorf("content slug %q is managed by another source", item.Slug)
		}

		tag, err := tx.Exec(ctx, `
			INSERT INTO content_items (slug, kind, status, comments_enabled, managed_by)
			VALUES ($1, $2, $3, $4, $5)
			ON CONFLICT (slug) DO UPDATE SET
				kind = EXCLUDED.kind,
				status = EXCLUDED.status,
				comments_enabled = EXCLUDED.comments_enabled,
				updated_at = now()
			WHERE content_items.managed_by = EXCLUDED.managed_by
			  AND (content_items.kind, content_items.status, content_items.comments_enabled)
			      IS DISTINCT FROM (EXCLUDED.kind, EXCLUDED.status, EXCLUDED.comments_enabled)
		`, item.Slug, item.Kind, item.Status, item.CommentsEnabled, snapshot.Source)
		if err != nil {
			return content.SyncResult{}, fmt.Errorf("upsert content item: %w", err)
		}
		result.Changed += tag.RowsAffected()
	}

	tag, err := tx.Exec(ctx, `
		UPDATE content_items
		SET status = 'archived', updated_at = now()
		WHERE managed_by = $1
		  AND NOT (slug = ANY($2::text[]))
		  AND status <> 'archived'
	`, snapshot.Source, slugs)
	if err != nil {
		return content.SyncResult{}, fmt.Errorf("archive absent content: %w", err)
	}
	result.Archived = tag.RowsAffected()

	if err := tx.Commit(ctx); err != nil {
		return content.SyncResult{}, fmt.Errorf("commit content sync: %w", err)
	}
	return result, nil
}
