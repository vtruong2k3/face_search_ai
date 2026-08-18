package http

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/http/handlers"
	"github.com/face-search-ai/api/internal/http/middleware"
	"github.com/face-search-ai/api/internal/ratelimit"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// SecurityControls carries the deliberate abuse and HTTP controls applied by the
// router: per-endpoint rate limiters, the per-request timeout, and the browser
// origin used for CORS. Zero-valued fields disable their control, which keeps the
// health-only NewRouter and tests simple.
type SecurityControls struct {
	WebOrigin      string
	RequestTimeout time.Duration
	AuthLimiter    *ratelimit.Limiter
	SearchLimiter  *ratelimit.Limiter
}

func NewRouter(checker handlers.Checker) http.Handler {
	return NewRouterWithAuth(checker, nil, nil, nil, nil, nil, nil, nil, SecurityControls{})
}

func NewRouterWithAuth(checker handlers.Checker, authHandler *handlers.Auth, authService *auth.Service, organizationsHandler *handlers.Organizations, eventsHandler *handlers.Events, photosHandler *handlers.Photos, searchHandler *handlers.Search, downloadsHandler *handlers.Downloads, controls SecurityControls) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health/live", handlers.Live)
	mux.HandleFunc("GET /health/ready", handlers.Ready(checker))
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /api/v1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"name": "face-search-api", "version": "v1"})
	})
	if authHandler != nil {
		// Registration, login, and refresh are credential-guessing and session-minting
		// surfaces, so they are rate limited per client address. Logout and me are
		// bounded by the caller's own session and left unthrottled.
		authLimited := func(handler http.HandlerFunc) http.Handler {
			return middleware.RateLimit(controls.AuthLimiter, middleware.ClientIPKey, "auth", handler)
		}
		mux.Handle("POST /api/v1/auth/register", authLimited(authHandler.Register))
		mux.Handle("POST /api/v1/auth/login", authLimited(authHandler.Login))
		mux.Handle("POST /api/v1/auth/refresh", authLimited(authHandler.Refresh))
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
		// Public selfie search is the most expensive and most sensitive public surface
		// (biometric inference + vector query), so it is rate limited per public token
		// combined with client address. The token path value is populated by the mux
		// before this wrapped handler runs.
		searchKey := func(r *http.Request) string { return r.PathValue("publicToken") + "|" + middleware.ClientIP(r) }
		mux.Handle("POST /api/v1/public/events/{publicToken}", middleware.RateLimit(controls.SearchLimiter, searchKey, "search", http.HandlerFunc(searchHandler.Public)))
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
	// Compose outermost first: every response (including CORS rejections, rate-limit
	// 429s, and timeout 503s) carries a request ID and the API security headers, and
	// is bounded by the per-request timeout. Metrics wraps the mux directly (innermost)
	// so the matched, bounded route template is available when the handler returns.
	handler := middleware.Metrics(mux)
	handler = middleware.Timeout(controls.RequestTimeout, handler)
	handler = middleware.CORS(controls.WebOrigin, handler)
	handler = middleware.RequestLog(handler)
	handler = middleware.SecurityHeaders(handler)
	handler = middleware.RequestID(handler)
	return handler
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
