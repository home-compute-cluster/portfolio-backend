package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/home-compute-cluster/portfolio-backend/internal/comments"
	commentspostgres "github.com/home-compute-cluster/portfolio-backend/internal/comments/postgres"
	"github.com/home-compute-cluster/portfolio-backend/internal/config"
	"github.com/home-compute-cluster/portfolio-backend/internal/content"
	contentpostgres "github.com/home-compute-cluster/portfolio-backend/internal/content/postgres"
	"github.com/home-compute-cluster/portfolio-backend/internal/httpapi"
	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/clock"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/postgres"
	"github.com/home-compute-cluster/portfolio-backend/internal/platform/visitor"
	"github.com/home-compute-cluster/portfolio-backend/internal/reactions"
	reactionspostgres "github.com/home-compute-cluster/portfolio-backend/internal/reactions/postgres"
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

	contentService := content.NewService(contentpostgres.NewStore(pool))
	commentService := comments.NewService(
		commentspostgres.NewStore(pool),
		contentService,
		comments.Limits{
			MaxAuthorChars:     cfg.Comments.MaxAuthorChars,
			MaxCommentChars:    cfg.Comments.MaxCommentChars,
			MaxCommentsPerPost: cfg.Comments.MaxCommentsPerPost,
			DefaultPageSize:    cfg.Comments.DefaultPageSize,
			MaximumPageSize:    cfg.Comments.MaximumPageSize,
		},
	)
	visitorIdentity := visitor.NewIdentity(cfg.Security.VisitorHMACKey)
	commentHandler := httpapi.NewCommentHandler(
		commentService,
		visitorIdentity,
		nil, // Rate-limiting assignment: wire only after its tagged acceptance suite passes.
		cfg.Database.QueryTimeout,
		logger,
	)
	viewService := reactions.NewViewService(
		reactionspostgres.NewViewStore(pool),
		contentService,
		clock.Real{},
		cfg.Views.DeduplicationWindow,
	)
	viewHandler := httpapi.NewViewHandler(
		viewService,
		visitorIdentity,
		nil, // Rate-limiting assignment: use a separate view allowance when implemented.
		cfg.Database.QueryTimeout,
		logger,
	)
	reactionStore := reactionspostgres.NewStore(pool)
	reactionHandler := httpapi.NewReactionHandler(
		reactions.NewLikeService(reactionStore, contentService),
		reactions.NewStatsService(reactionStore, contentService),
		visitorIdentity,
		nil, // Rate-limiting assignment: wire the like allowance after implementation.
		cfg.Database.QueryTimeout,
		logger,
	)
	clientIP := httpmiddleware.NewClientIPResolver(cfg.Security.TrustedProxyCIDRs)

	server := &http.Server{
		Addr: cfg.HTTP.Address,
		Handler: httpapi.NewApplicationRouter(
			pool,
			cfg.Database.ReadinessTimeout,
			logger,
			httpapi.FeatureHandlers{
				ClientIP:  clientIP,
				Comments:  commentHandler,
				Views:     viewHandler,
				Reactions: reactionHandler,
			},
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
