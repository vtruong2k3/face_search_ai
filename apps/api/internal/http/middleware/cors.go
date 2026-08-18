package middleware

import "net/http"

func CORS(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		if requestOrigin != "" && requestOrigin != origin {
			http.Error(w, "origin rejected", http.StatusForbidden)
			return
		}
		if requestOrigin == origin && origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Vary", "Origin")
		}
		if r.Method == http.MethodOptions {
			// The authenticated web app performs cross-origin GET/POST/PATCH/DELETE
			// (e.g. Event update and archive) with credentials. Advertise exactly the
			// methods and headers the app uses so legitimate preflights succeed while
			// the origin allow-list above still gates who may send them.
			w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
