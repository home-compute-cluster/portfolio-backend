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

func (store *Store) PublishedPostExists(ctx context.Context, slug string) (bool, error) {
	var exists bool
	err := store.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM content_items
			WHERE slug = $1 AND kind = 'post' AND status = 'published'
		)
	`, slug).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check published post: %w", err)
	}

	return exists, nil
}
