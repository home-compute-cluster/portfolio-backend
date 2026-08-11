package middleware

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime/debug"

	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// Recoverer converts handler panics into an internal error response and logs
// diagnostic details without exposing them to the client.
func Recoverer(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if recovered := recover(); recovered != nil {
					logger.Error(
						"panic recovered",
						"request_id", chimiddleware.GetReqID(r.Context()),
						"method", r.Method,
						"path", r.URL.Path,
						"error_category", "panic",
						"stack", string(debug.Stack()),
					)
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.Header().Set("Cache-Control", "no-store")
					w.WriteHeader(http.StatusInternalServerError)
					_ = json.NewEncoder(w).Encode(struct {
						Error string `json:"error"`
					}{Error: "internal_error"})
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
