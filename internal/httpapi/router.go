package httpapi

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

func NewRouter(
	database DatabasePinger,
	readinessTimeout time.Duration,
) http.Handler {
	health := NewHealthHandler(
		database,
		readinessTimeout,
	)

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)

	router.Get("api/healthz", health.Healthz)
	router.Get("api/readyz", health.Readyz)

	return router
}
