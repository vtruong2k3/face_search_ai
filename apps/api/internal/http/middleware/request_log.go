package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

func RequestLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request", "request_id", RequestIDFromContext(r.Context()), "method", r.Method, "path", r.URL.Path, "duration", time.Since(started))
	})
}
