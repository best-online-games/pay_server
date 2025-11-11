package middlewarehelpers

import (
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"
)

// RequestLogger writes basic request info to the provided logger.
func RequestLogger(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			logger.Info("http request started",
				"method", r.Method,
				"path", r.URL.Path,
				"remote", r.RemoteAddr,
				"host", r.Host,
			)

			next.ServeHTTP(w, r)

			logger.Info("http request finished",
				"method", r.Method,
				"path", r.URL.Path,
				"duration", time.Since(start),
			)
		})
	}
}

// Recover safely catches panics and logs stack trace.
func Recover(logger *slog.Logger) middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					logger.Error("panic recovered",
						"method", r.Method,
						"path", r.URL.Path,
						"panic", rec,
						"stack", string(debug.Stack()),
					)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}
