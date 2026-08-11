package middleware

import (
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
)

// RequestLogger records one bounded structured event after each HTTP request.
func RequestLogger(logger *slog.Logger) func(http.Handler) http.Handler {
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			started := time.Now()
			wrapped := chimiddleware.NewWrapResponseWriter(response, request.ProtoMajor)
			next.ServeHTTP(wrapped, request)

			status := wrapped.Status()
			if status == 0 {
				status = http.StatusOK
			}
			route := chi.RouteContext(request.Context()).RoutePattern()
			if route == "" || route == "/*" {
				route = "unmatched"
			}
			logger.Info(
				"HTTP request completed",
				"request_id", chimiddleware.GetReqID(request.Context()),
				"method", request.Method,
				"route", route,
				"status", status,
				"duration", time.Since(started),
				"response_bytes", wrapped.BytesWritten(),
				"error_category", responseCategory(status),
			)
		})
	}
}

func responseCategory(status int) string {
	switch {
	case status >= 500:
		return "server_error"
	case status >= 400:
		return "client_error"
	default:
		return "none"
	}
}
