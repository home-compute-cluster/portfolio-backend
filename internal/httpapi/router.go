package httpapi

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	httpmiddleware "github.com/home-compute-cluster/portfolio-backend/internal/httpapi/middleware"
)

func NewRouter(
	database DatabasePinger,
	readinessTimeout time.Duration,
	logger *slog.Logger,
) http.Handler {
	return newRouter(database, readinessTimeout, logger, nil)
}

type FeatureHandlers struct {
	ClientIP *httpmiddleware.ClientIPResolver
	Comments *CommentHandler
	Views    *ViewHandler
}

func NewApplicationRouter(
	database DatabasePinger,
	readinessTimeout time.Duration,
	logger *slog.Logger,
	features FeatureHandlers,
) http.Handler {
	return newRouter(database, readinessTimeout, logger, func(router chi.Router) {
		if features.ClientIP == nil || (features.Comments == nil && features.Views == nil) {
			return
		}
		router.Route("/api/posts/{slug}", func(posts chi.Router) {
			posts.Use(features.ClientIP.Middleware)
			if features.Comments != nil {
				posts.Get("/comments", features.Comments.List)
				posts.Post("/comments", features.Comments.Create)
			}
			if features.Views != nil {
				posts.Post("/view", features.Views.Record)
			}
		})
	})
}

func newRouter(
	database DatabasePinger,
	readinessTimeout time.Duration,
	logger *slog.Logger,
	additionalRoutes func(chi.Router),
) http.Handler {
	health := NewHealthHandler(database, readinessTimeout, logger)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(httpmiddleware.Recoverer(logger))

	router.Get("/api/healthz", health.Healthz)
	router.Get("/api/readyz", health.Readyz)

	if additionalRoutes != nil {
		additionalRoutes(router)
	}

	return router
}
