package postgres

import (
	"context"
	"fmt"

	"github.com/home-compute-cluster/portfolio-backend/internal/reactions"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store persists likes and reads public reaction totals from PostgreSQL.
type Store struct {
	pool *pgxpool.Pool
}

// NewStore constructs the PostgreSQL store for likes and public statistics.
func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// AddLike inserts the visitor's unique like and reports whether a row was added.
func (store *Store) AddLike(
	ctx context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	var changed bool
	err := store.pool.QueryRow(ctx, `
		WITH inserted AS (
			INSERT INTO post_likes (post_slug, visitor_hash)
			VALUES ($1, $2)
			ON CONFLICT (post_slug, visitor_hash) DO NOTHING
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM inserted)
	`, postSlug, visitorHash[:]).Scan(&changed)
	if err != nil {
		return false, fmt.Errorf("add post like: %w", err)
	}
	return changed, nil
}

// RemoveLike deletes the visitor's like and reports whether a row was removed.
func (store *Store) RemoveLike(
	ctx context.Context,
	postSlug string,
	visitorHash [32]byte,
) (bool, error) {
	var changed bool
	err := store.pool.QueryRow(ctx, `
		WITH deleted AS (
			DELETE FROM post_likes
			WHERE post_slug = $1 AND visitor_hash = $2
			RETURNING 1
		)
		SELECT EXISTS (SELECT 1 FROM deleted)
	`, postSlug, visitorHash[:]).Scan(&changed)
	if err != nil {
		return false, fmt.Errorf("remove post like: %w", err)
	}
	return changed, nil
}

// GetStats reads the cached view count and derives the like count in one statement snapshot.
func (store *Store) GetStats(ctx context.Context, postSlug string) (reactions.Stats, error) {
	var stats reactions.Stats
	err := store.pool.QueryRow(ctx, `
		SELECT
			COALESCE((SELECT view_count FROM post_stats WHERE post_slug = $1), 0),
			(SELECT count(*) FROM post_likes WHERE post_slug = $1)
	`, postSlug).Scan(&stats.Views, &stats.Likes)
	if err != nil {
		return reactions.Stats{}, fmt.Errorf("read post statistics: %w", err)
	}
	return stats, nil
}
