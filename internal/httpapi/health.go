package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"time"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

type HealthHandler struct {
	database         DatabasePinger
	readinessTimeout time.Duration
}

func NewHealthHandler(
	database DatabasePinger,
	readinessTimeout time.Duration,
) *HealthHandler {
	return &HealthHandler{
		database:         database,
		readinessTimeout: readinessTimeout,
	}
}

// Healthz checks only whether the HTTP process is alive.
//
// Do not query PostgreSQL here. A database outage should not cause
// Kubernetes to restart an otherwise healthy API process.
func (h *HealthHandler) Healthz(
	w http.ResponseWriter,
	_ *http.Request,
) {
	writeStatus(w, http.StatusOK, "ok")
}

// Readyz checks whether the dependencies required to serve traffic
// are currently available.
func (h *HealthHandler) Readyz(
	w http.ResponseWriter,
	r *http.Request,
) {
	if h.database == nil {
		writeStatus(
			w,
			http.StatusServiceUnavailable,
			"unavailable",
		)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.readinessTimeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		writeStatus(
			w,
			http.StatusServiceUnavailable,
			"unavailable",
		)
		return
	}

	writeStatus(w, http.StatusOK, "ok")
}

func writeStatus(w http.ResponseWriter, statusCode int, status string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{
		Status: status,
	})

}
