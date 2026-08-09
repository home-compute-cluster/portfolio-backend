package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/home-compute-cluster/portfolio-backend/internal/config"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/migrate"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	os.Exit(execute(os.Args[1:], logger))
}

func execute(args []string, logger *slog.Logger) int {
	flags := flag.NewFlagSet("migrate", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	migrationDir := flags.String("dir", "migrations", "directory containing versioned SQL migrations")
	if err := flags.Parse(args); err != nil {
		logger.Error("parse migration arguments", "error", err)
		return 2
	}

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		logger.Error("load .env file", "error", err)
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		logger.Error("load configuration", "error", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, *migrationDir, logger); err != nil {
		logger.Error("migration failed", "error", err)
		return 1
	}

	return 0
}

func run(
	ctx context.Context,
	cfg config.Config,
	migrationDir string,
	logger *slog.Logger,
) error {
	connectCtx, cancelConnect := context.WithTimeout(ctx, cfg.Database.ConnectTimeout)
	pool, err := postgres.Open(connectCtx, cfg.Database)
	cancelConnect()
	if err != nil {
		return fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	defer pool.Close()

	if err := migrate.Up(ctx, pool, os.DirFS(migrationDir), logger); err != nil {
		return err
	}

	logger.Info("database schema is current")

	return nil
}
