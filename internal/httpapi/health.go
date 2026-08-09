package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type DatabasePinger interface {
	Ping(context.Context) error
}

type HealthHandler struct {
	database         DatabasePinger
	readinessTimeout time.Duration
	logger           *slog.Logger
}

func NewHealthHandler(
	database DatabasePinger,
	readinessTimeout time.Duration,
	logger *slog.Logger,
) *HealthHandler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return &HealthHandler{
		database:         database,
		readinessTimeout: readinessTimeout,
		logger:           logger,
	}
}

// Healthz reports process liveness and deliberately does not query PostgreSQL.
func (h *HealthHandler) Healthz(w http.ResponseWriter, _ *http.Request) {
	writeStatus(w, http.StatusOK, "ok")
}

// Readyz reports whether PostgreSQL is reachable within the configured timeout.
func (h *HealthHandler) Readyz(w http.ResponseWriter, r *http.Request) {
	if h.database == nil {
		h.logger.Error("readiness check failed", "error", "database pinger is not configured")
		writeStatus(w, http.StatusServiceUnavailable, "unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), h.readinessTimeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		h.logger.Warn("readiness check failed", "error", err)
		writeStatus(w, http.StatusServiceUnavailable, "unavailable")
		return
	}

	writeStatus(w, http.StatusOK, "ready")
}

func writeStatus(w http.ResponseWriter, statusCode int, status string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(statusCode)

	_ = json.NewEncoder(w).Encode(struct {
		Status string `json:"status"`
	}{
		Status: status,
	})
}
