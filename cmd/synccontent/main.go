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
	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	contentpostgres "github.com/home-compute-cluster/portfolio-backend/internal/content/postgres"
	platformpostgres "github.com/home-compute-cluster/portfolio-backend/internal/platform/postgres"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	os.Exit(execute(os.Args[1:], logger))
}

func execute(args []string, logger *slog.Logger) int {
	flags := flag.NewFlagSet("sync-content", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	manifestPath := flags.String("manifest", "", "path to the complete frontend content manifest")
	if err := flags.Parse(args); err != nil || *manifestPath == "" {
		logger.Error("parse content sync arguments", "error", "-manifest is required")
		return 2
	}

	manifestFile, err := os.Open(*manifestPath)
	if err != nil {
		logger.Error("open content manifest", "error", err)
		return 1
	}
	snapshot, readErr := content.ReadManifest(manifestFile)
	closeErr := manifestFile.Close()
	if readErr != nil {
		logger.Error("validate content manifest", "error", readErr)
		return 1
	}
	if closeErr != nil {
		logger.Error("close content manifest", "error", closeErr)
		return 1
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
	if err := run(ctx, cfg, snapshot, logger); err != nil {
		logger.Error("content sync failed", "error", err)
		return 1
	}
	return 0
}

// run connects to PostgreSQL and applies the already validated manifest.
func run(ctx context.Context, cfg config.Config, snapshot content.Snapshot, logger *slog.Logger) error {
	connectCtx, cancelConnect := context.WithTimeout(ctx, cfg.Database.ConnectTimeout)
	pool, err := platformpostgres.Open(connectCtx, cfg.Database)
	cancelConnect()
	if err != nil {
		return fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	defer pool.Close()

	result, err := content.NewSyncService(contentpostgres.NewStore(pool)).Sync(ctx, snapshot)
	if err != nil {
		return err
	}
	logger.Info(
		"content registry synchronized",
		"source", snapshot.Source,
		"revision", snapshot.Revision,
		"items", len(snapshot.Items),
		"changed", result.Changed,
		"archived", result.Archived,
	)
	return nil
}
