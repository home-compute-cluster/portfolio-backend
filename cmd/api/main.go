package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"

	"github.com/home-compute-cluster/portfolio-backend/internal/app"
	"github.com/home-compute-cluster/portfolio-backend/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := godotenv.Load(); err != nil && !os.IsNotExist(err) {
		logger.Error("load .env file", "error", err)
		os.Exit(1)
	}

	cfg, err := config.LoadAPI()
	if err != nil {
		logger.Error("load configuration", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := app.Run(ctx, cfg, logger); err != nil {
		logger.Error("application stopped", "error", fmt.Errorf("run API: %w", err))
		os.Exit(1)
	}
}
