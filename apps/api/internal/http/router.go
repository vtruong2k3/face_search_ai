package http

import (
	"encoding/json"
	"net/http"

	"github.com/face-search-ai/api/internal/http/handlers"
	"github.com/face-search-ai/api/internal/http/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(checker handlers.Checker) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", handlers.Live)
	mux.HandleFunc("GET /health/ready", handlers.Ready(checker))
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /api/v1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"name": "face-search-api", "version": "v1"})
	})
	return middleware.RequestLog(mux)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
