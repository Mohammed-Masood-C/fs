package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// AccessLog logs one line per request once its response is complete.
func AccessLog(logger *slog.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := record(w)
			defer func() {
				logger.LogAttrs(r.Context(), slog.LevelInfo, "request",
					slog.String("remote", r.RemoteAddr),
					slog.String("method", r.Method),
					slog.String("uri", r.URL.RequestURI()),
					slog.String("proto", r.Proto),
					slog.Int("status", rec.Status()),
					slog.Int64("bytes", rec.bytes),
					slog.Duration("duration", time.Since(start)),
					slog.String("user_agent", r.UserAgent()),
				)
			}()
			next.ServeHTTP(rec, r)
		})
	}
}
