package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	"github.com/jackc/pgx/v5"
)

func (store *Store) ListForModeration(
	ctx context.Context,
	status comments.Status,
	beforeID int64,
	limit int,
) ([]comments.Comment, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, post_slug, author_name, body, status, created_at, hidden_at
		FROM comments
		WHERE ($1::text = '' OR status = $1)
		  AND ($2::bigint = 0 OR id < $2)
		ORDER BY id DESC
		LIMIT $3
	`, status, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list comments for moderation: %w", err)
	}
	defer rows.Close()

	result := make([]comments.Comment, 0, limit)
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan moderation comment: %w", err)
		}
		result = append(result, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate moderation comments: %w", err)
	}
	return result, nil
}

func (store *Store) SetVisibility(
	ctx context.Context,
	id int64,
	desired comments.Status,
	maxVisible int,
) (comments.Comment, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return comments.Comment{}, fmt.Errorf("begin comment moderation: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	var postSlug string
	if err := tx.QueryRow(ctx, `SELECT post_slug FROM comments WHERE id = $1`, id).Scan(&postSlug); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return comments.Comment{}, comments.ErrNotFound
		}
		return comments.Comment{}, fmt.Errorf("find comment for moderation: %w", err)
	}
	if _, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock($1, hashtext($2))`,
		commentLockNamespace,
		postSlug,
	); err != nil {
		return comments.Comment{}, fmt.Errorf("lock post comments for moderation: %w", err)
	}

	current, err := scanComment(tx.QueryRow(ctx, `
		SELECT id, post_slug, author_name, body, status, created_at, hidden_at
		FROM comments
		WHERE id = $1
		FOR UPDATE
	`, id))
	if err != nil {
		return comments.Comment{}, fmt.Errorf("read comment for moderation: %w", err)
	}
	if current.Status == desired {
		if err := tx.Commit(ctx); err != nil {
			return comments.Comment{}, fmt.Errorf("commit idempotent moderation: %w", err)
		}
		return current, nil
	}

	if desired == comments.StatusVisible {
		var visibleCount int
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM comments WHERE post_slug = $1 AND status = 'visible'
		`, postSlug).Scan(&visibleCount); err != nil {
			return comments.Comment{}, fmt.Errorf("count comments before unhide: %w", err)
		}
		if visibleCount >= maxVisible {
			return comments.Comment{}, comments.ErrPostFull
		}
	}

	updated, err := scanComment(tx.QueryRow(ctx, `
		UPDATE comments
		SET status = $2,
		    hidden_at = CASE WHEN $2 = 'hidden' THEN now() ELSE NULL END
		WHERE id = $1
		RETURNING id, post_slug, author_name, body, status, created_at, hidden_at
	`, id, desired))
	if err != nil {
		return comments.Comment{}, fmt.Errorf("update comment visibility: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return comments.Comment{}, fmt.Errorf("commit comment moderation: %w", err)
	}
	return updated, nil
}
