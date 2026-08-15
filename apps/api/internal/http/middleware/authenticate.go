package middleware

import (
	"net/http"
	"strings"

	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/domain/authorization"
)

func Authenticate(service *auth.Service, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := r.Header.Get("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			writeRejected(w)
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		if token == "" || strings.ContainsAny(token, " \t\r\n") {
			writeRejected(w)
			return
		}
		userID, err := service.ParseAccess(token)
		if err != nil {
			writeRejected(w)
			return
		}
		ctx := authorization.WithPrincipal(r.Context(), authorization.Principal{UserID: userID})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func writeRejected(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("{\"code\":\"authentication_rejected\",\"message\":\"Authentication request rejected.\"}\n"))
}
