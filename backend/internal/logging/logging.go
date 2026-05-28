// Package logging configures the application logger and HTTP middleware.
package logging

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func New(level slog.Level, env string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: level}
	if env == "dev" {
		return slog.New(slog.NewTextHandler(os.Stdout, opts))
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, opts))
}

type ctxKey struct{}

func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// Middleware logs each request with method, path, status, duration, and request id.
func Middleware(base *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)

			reqID := middleware.GetReqID(r.Context())
			l := base.With("request_id", reqID)
			ctx := WithLogger(r.Context(), l)

			next.ServeHTTP(ww, r.WithContext(ctx))

			l.Info("http",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.Status(),
				"bytes", ww.BytesWritten(),
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}
