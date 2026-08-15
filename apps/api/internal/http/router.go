package http

import (
	"encoding/json"
	"net/http"

	"github.com/face-search-ai/api/internal/http/handlers"
	"github.com/face-search-ai/api/internal/http/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(checker handlers.Checker) http.Handler {
	return NewRouterWithAuth(checker, nil, "")
}

func NewRouterWithAuth(checker handlers.Checker, authHandler *handlers.Auth, webOrigin string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", handlers.Live)
	mux.HandleFunc("GET /health/ready", handlers.Ready(checker))
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /api/v1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"name": "face-search-api", "version": "v1"})
	})
	if authHandler != nil {
		mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
		mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
		mux.HandleFunc("POST /api/v1/auth/refresh", authHandler.Refresh)
		mux.HandleFunc("POST /api/v1/auth/logout", authHandler.Logout)
		mux.HandleFunc("GET /api/v1/auth/me", authHandler.Me)
	}
	return middleware.CORS(webOrigin, middleware.RequestLog(mux))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
