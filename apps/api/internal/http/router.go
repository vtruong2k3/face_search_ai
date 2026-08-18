package http

import (
	"encoding/json"
	"net/http"

	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/http/handlers"
	"github.com/face-search-ai/api/internal/http/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(checker handlers.Checker) http.Handler {
	return NewRouterWithAuth(checker, nil, nil, nil, nil, nil, nil, nil, "")
}

func NewRouterWithAuth(checker handlers.Checker, authHandler *handlers.Auth, authService *auth.Service, organizationsHandler *handlers.Organizations, eventsHandler *handlers.Events, photosHandler *handlers.Photos, searchHandler *handlers.Search, downloadsHandler *handlers.Downloads, webOrigin string) http.Handler {
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
	if authService != nil && organizationsHandler != nil {
		mux.Handle("GET /api/v1/organizations", middleware.Authenticate(authService, http.HandlerFunc(organizationsHandler.List)))
		mux.Handle("GET /api/v1/organizations/{organizationId}/membership", middleware.Authenticate(authService, http.HandlerFunc(organizationsHandler.Membership)))
	}
	if eventsHandler != nil {
		mux.HandleFunc("GET /api/v1/public/events/{publicToken}", eventsHandler.Public)
	}
	if searchHandler != nil {
		mux.HandleFunc("POST /api/v1/public/events/{publicToken}", searchHandler.Public)
	}
	if downloadsHandler != nil {
		mux.HandleFunc("POST /api/v1/public/events/{publicToken}/downloads", downloadsHandler.Public)
	}
	if authService != nil && eventsHandler != nil {
		protected := func(handler http.HandlerFunc) http.Handler { return middleware.Authenticate(authService, handler) }
		mux.Handle("GET /api/v1/organizations/{organizationId}/events", protected(eventsHandler.List))
		mux.Handle("POST /api/v1/organizations/{organizationId}/events", protected(eventsHandler.Create))
		mux.Handle("GET /api/v1/organizations/{organizationId}/events/{eventId}", protected(eventsHandler.Get))
		mux.Handle("PATCH /api/v1/organizations/{organizationId}/events/{eventId}", protected(eventsHandler.Update))
		mux.Handle("DELETE /api/v1/organizations/{organizationId}/events/{eventId}", protected(eventsHandler.Archive))
		mux.Handle("GET /api/v1/organizations/{organizationId}/events/{eventId}/status", protected(eventsHandler.Status))
	}
	if authService != nil && photosHandler != nil {
		protected := func(handler http.HandlerFunc) http.Handler { return middleware.Authenticate(authService, handler) }
		base := "/api/v1/organizations/{organizationId}/events/{eventId}/photos"
		mux.Handle("GET "+base, protected(photosHandler.List))
		mux.Handle("POST "+base, protected(photosHandler.Create))
		mux.Handle("GET "+base+"/{photoId}", protected(photosHandler.Get))
		mux.Handle("DELETE "+base+"/{photoId}", protected(photosHandler.Delete))
		mux.Handle("POST "+base+"/{photoId}/uploads", protected(photosHandler.InitiateUpload))
		mux.Handle("POST "+base+"/{photoId}/uploads/parts/{partNumber}", protected(photosHandler.SignUploadPart))
		mux.Handle("POST "+base+"/{photoId}/uploads/complete", protected(photosHandler.CompleteUpload))
		mux.Handle("POST "+base+"/{photoId}/uploads/abort", protected(photosHandler.AbortUpload))
		mux.Handle("POST "+base+"/{photoId}/reprocess", protected(photosHandler.Reprocess))
	}
	return middleware.CORS(webOrigin, middleware.RequestID(middleware.RequestLog(mux)))
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
