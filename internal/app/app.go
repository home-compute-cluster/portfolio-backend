package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/config"
	"github.com/home-compute-cluster/portfolio-backend/internal/httpapi"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/postgres"
)

// Run constructs and serves the API until ctx is cancelled or the server fails.
func Run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	startupCtx, cancelStartup := context.WithTimeout(ctx, cfg.Database.ConnectTimeout)
	pool, err := postgres.Open(startupCtx, cfg.Database)
	cancelStartup()
	if err != nil {
		return fmt.Errorf("open PostgreSQL pool: %w", err)
	}
	defer pool.Close()

	server := &http.Server{
		Addr: cfg.HTTP.Address,
		Handler: httpapi.NewRouter(
			pool,
			cfg.Database.ReadinessTimeout,
			logger,
		),
		ReadHeaderTimeout: cfg.HTTP.ReadHeaderTimeout,
		ReadTimeout:       cfg.HTTP.ReadTimeout,
		WriteTimeout:      cfg.HTTP.WriteTimeout,
		IdleTimeout:       cfg.HTTP.IdleTimeout,
	}

	listener, err := net.Listen("tcp", cfg.HTTP.Address)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTP.Address, err)
	}

	logger.Info("HTTP server listening", "address", listener.Addr().String())

	return serve(ctx, server, listener, cfg.HTTP.ShutdownTimeout, logger)
}

func serve(
	ctx context.Context,
	server *http.Server,
	listener net.Listener,
	shutdownTimeout time.Duration,
	logger *slog.Logger,
) error {
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.Serve(listener)
	}()

	select {
	case err := <-serverErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}

		return fmt.Errorf("serve HTTP: %w", err)
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	}

	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		shutdownTimeout,
	)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownCtx); err != nil {
		_ = server.Close()
		return fmt.Errorf("gracefully shut down HTTP server: %w", err)
	}

	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("stop HTTP server: %w", err)
	}

	logger.Info("HTTP server stopped")

	return nil
}
