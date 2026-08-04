package postgres

import (
	"context"
	"fmt"

	"github.com/home-compute-cluster/portfolio-backend/internal/config"
	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(
	ctx context.Context,
	cfg config.DatabaseConfig,
) (*pgxpool.Pool, error) {
	poolConfig, err := pgxpool.ParseConfig(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("parse DATABASE_URL: %w", err)
	}

	poolConfig.MaxConns = cfg.MaxConns
	poolConfig.MinConns = cfg.MinConns
	poolConfig.ConnConfig.ConnectTimeout = cfg.ConnectTimeout

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create PostgreSQL pool: %w", err)
	}

	return pool, nil
}
