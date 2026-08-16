package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

// PublishedContentExists reports whether slug is explicitly registered and
// published, regardless of its descriptive content kind.
func (store *Store) PublishedContentExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := store.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM content_items
			WHERE slug = $1 AND status = 'published'
		)
	`, slug).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check published content: %w", err)
	}

	return exists, nil
}
