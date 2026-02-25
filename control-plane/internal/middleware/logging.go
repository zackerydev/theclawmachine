package middleware

import (
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// statusRecorder captures the response status code.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// RequestLogger logs every HTTP request with method, path, status, and duration.
// Static asset requests are logged at DEBUG level to reduce noise.
func RequestLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}

		next.ServeHTTP(rec, r)

		duration := time.Since(start)
		path := r.URL.Path

		// Static assets and health checks at debug level
		if strings.HasPrefix(path, "/static/") || path == "/health" {
			slog.Debug("request",
				"method", r.Method,
				"path", path,
				"status", rec.status,
				"duration", duration.Round(time.Millisecond).String(),
			)
			return
		}

		level := slog.LevelInfo
		if rec.status >= 500 {
			level = slog.LevelError
		} else if rec.status >= 400 {
			level = slog.LevelWarn
		}

		slog.Log(r.Context(), level, "request",
			"method", r.Method,
			"path", path,
			"status", rec.status,
			"duration", duration.Round(time.Millisecond).String(),
		)
	})
}
