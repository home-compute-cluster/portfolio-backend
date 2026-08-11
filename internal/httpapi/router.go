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
	ClientIP      *httpmiddleware.ClientIPResolver
	AdminAccess   func(http.Handler) http.Handler
	AdminComments *AdminCommentHandler
	Comments      *CommentHandler
	Views         *ViewHandler
	Reactions     *ReactionHandler
}

func NewApplicationRouter(
	database DatabasePinger,
	readinessTimeout time.Duration,
	logger *slog.Logger,
	features FeatureHandlers,
) http.Handler {
	return newRouter(database, readinessTimeout, logger, func(router chi.Router) {
		if features.Comments != nil || features.Views != nil || features.Reactions != nil {
			router.Route("/api/posts/{slug}", func(posts chi.Router) {
				if features.Comments != nil {
					posts.Get("/comments", features.Comments.List)
				}
				if features.Reactions != nil {
					posts.Get("/stats", features.Reactions.Stats)
				}
				if features.ClientIP != nil {
					posts.Group(func(writes chi.Router) {
						writes.Use(features.ClientIP.Middleware)
						if features.Comments != nil {
							writes.Post("/comments", features.Comments.Create)
						}
						if features.Views != nil {
							writes.Post("/view", features.Views.Record)
						}
						if features.Reactions != nil {
							writes.Put("/like", features.Reactions.Like)
							writes.Delete("/like", features.Reactions.Unlike)
						}
					})
				}
			})
		}
		if features.AdminAccess != nil && features.AdminComments != nil {
			router.Route("/api/admin", func(admin chi.Router) {
				admin.Use(features.AdminAccess)
				admin.Get("/comments", features.AdminComments.List)
				admin.Post("/comments/{id}/hide", features.AdminComments.Hide)
				admin.Post("/comments/{id}/unhide", features.AdminComments.Unhide)
			})
		}
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
	router.Use(httpmiddleware.RequestLogger(logger))
	router.Use(httpmiddleware.Recoverer(logger))
	router.MethodNotAllowed(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusMethodNotAllowed)
	})

	router.Get("/api/healthz", health.Healthz)
	router.Get("/api/readyz", health.Readyz)

	if additionalRoutes != nil {
		additionalRoutes(router)
	}
	// A final wildcard keeps unmatched requests inside the normal middleware chain.
	router.Handle("/*", http.NotFoundHandler())

	return router
}
