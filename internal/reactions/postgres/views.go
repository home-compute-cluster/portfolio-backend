package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type ViewStore struct {
	pool *pgxpool.Pool
}

func NewViewStore(pool *pgxpool.Pool) *ViewStore {
	return &ViewStore{pool: pool}
}

func (store *ViewStore) RecordView(
	ctx context.Context,
	postSlug string,
	visitorHash [32]byte,
	countedAt time.Time,
	deduplicationWindow time.Duration,
) (bool, error) {
	cutoff := countedAt.Add(-deduplicationWindow)
	var counted bool
	err := store.pool.QueryRow(ctx, `
		WITH counted AS (
			INSERT INTO post_view_visitors (post_slug, visitor_hash, last_counted_at)
			VALUES ($1, $2, $3)
			ON CONFLICT (post_slug, visitor_hash) DO UPDATE
			SET last_counted_at = EXCLUDED.last_counted_at
			WHERE post_view_visitors.last_counted_at <= $4
			RETURNING post_slug
		), stats_update AS (
			INSERT INTO post_stats (post_slug, view_count)
			SELECT post_slug, 1 FROM counted
			ON CONFLICT (post_slug) DO UPDATE
			SET view_count = post_stats.view_count + 1
			RETURNING post_slug
		)
		SELECT EXISTS (SELECT 1 FROM counted)
	`, postSlug, visitorHash[:], countedAt, cutoff).Scan(&counted)
	if err != nil {
		return false, fmt.Errorf("record post view: %w", err)
	}
	return counted, nil
}

func (store *ViewStore) Count(ctx context.Context, postSlug string) (int64, error) {
	var count int64
	err := store.pool.QueryRow(ctx, `
		SELECT COALESCE((SELECT view_count FROM post_stats WHERE post_slug = $1), 0)
	`, postSlug).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("read post view count: %w", err)
	}
	return count, nil
}
