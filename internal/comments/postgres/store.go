package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const commentLockNamespace int32 = 0x434d4e54

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) *Store {
	return &Store{pool: pool}
}

func (store *Store) CreateVisibleIfUnderLimit(
	ctx context.Context,
	input comments.CreateInput,
	maxVisible int,
) (comments.Comment, error) {
	tx, err := store.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return comments.Comment{}, fmt.Errorf("begin comment creation: %w", err)
	}
	defer func() {
		_ = tx.Rollback(context.Background())
	}()

	// The per-post transaction lock makes the visible-count check and insertion
	// one correctness boundary, including when concurrent requests hit one post.
	if _, err := tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock($1, hashtext($2))`,
		commentLockNamespace,
		input.PostSlug,
	); err != nil {
		return comments.Comment{}, fmt.Errorf("lock post comments: %w", err)
	}

	// Lock the registry row while checking policy. A concurrent manifest sync
	// can therefore linearize either before or after this comment, but cannot
	// disable comments between this check and the insert.
	var commentsAvailable bool
	err = tx.QueryRow(ctx, `
		SELECT status = 'published' AND comments_enabled
		FROM content_items
		WHERE slug = $1
		FOR SHARE
	`, input.PostSlug).Scan(&commentsAvailable)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !commentsAvailable) {
		return comments.Comment{}, comments.ErrUnavailable
	}
	if err != nil {
		return comments.Comment{}, fmt.Errorf("check comment availability: %w", err)
	}

	var visibleCount int
	if err := tx.QueryRow(ctx, `
		SELECT count(*)
		FROM comments
		WHERE post_slug = $1 AND status = 'visible'
	`, input.PostSlug).Scan(&visibleCount); err != nil {
		return comments.Comment{}, fmt.Errorf("count visible comments: %w", err)
	}
	if visibleCount >= maxVisible {
		return comments.Comment{}, comments.ErrPostFull
	}

	comment, err := scanComment(tx.QueryRow(ctx, `
		INSERT INTO comments (post_slug, author_name, body, visitor_hash)
		VALUES ($1, $2, $3, $4)
		RETURNING id, post_slug, author_name, body, status, created_at, hidden_at
	`, input.PostSlug, input.AuthorName, input.Body, input.VisitorHash[:]))
	if err != nil {
		return comments.Comment{}, fmt.Errorf("insert comment: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return comments.Comment{}, fmt.Errorf("commit comment creation: %w", err)
	}

	return comment, nil
}

func (store *Store) ListVisible(
	ctx context.Context,
	postSlug string,
	beforeID int64,
	limit int,
) ([]comments.Comment, error) {
	rows, err := store.pool.Query(ctx, `
		SELECT id, post_slug, author_name, body, status, created_at, hidden_at
		FROM comments
		WHERE post_slug = $1
		  AND status = 'visible'
		  AND ($2::bigint = 0 OR id < $2)
		ORDER BY id DESC
		LIMIT $3
	`, postSlug, beforeID, limit)
	if err != nil {
		return nil, fmt.Errorf("list visible comments: %w", err)
	}
	defer rows.Close()

	result := make([]comments.Comment, 0, limit)
	for rows.Next() {
		comment, err := scanComment(rows)
		if err != nil {
			return nil, fmt.Errorf("scan visible comment: %w", err)
		}
		result = append(result, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate visible comments: %w", err)
	}

	return result, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanComment(row rowScanner) (comments.Comment, error) {
	var comment comments.Comment
	err := row.Scan(
		&comment.ID,
		&comment.PostSlug,
		&comment.AuthorName,
		&comment.Body,
		&comment.Status,
		&comment.CreatedAt,
		&comment.HiddenAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return comments.Comment{}, comments.ErrNotFound
	}

	return comment, err
}
