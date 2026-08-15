package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/face-search-ai/api/internal/domain/auth"
)

type Auth struct {
	service      *auth.Service
	cookieSecure bool
	refreshTTL   time.Duration
}

func NewAuth(service *auth.Service, secure bool, ttl time.Duration) *Auth {
	return &Auth{service: service, cookieSecure: secure, refreshTTL: ttl}
}

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type authResponse struct {
	AccessToken string    `json:"accessToken"`
	TokenType   string    `json:"tokenType"`
	ExpiresIn   int64     `json:"expiresIn"`
	User        auth.User `json:"user"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid request.")
		return false
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "Invalid request.")
		return false
	}
	return true
}
func writeAuthJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func writeAuthError(w http.ResponseWriter, status int, code, message string) {
	writeAuthJSON(w, status, map[string]string{"code": code, "message": message})
}
func (h *Auth) setCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{Name: "face_search_refresh", Value: token, Path: "/api/v1/auth", HttpOnly: true, Secure: h.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}
func (h *Auth) respond(w http.ResponseWriter, status int, result auth.Result) {
	h.setCookie(w, result.RefreshToken, int(h.refreshTTL.Seconds()))
	writeAuthJSON(w, status, authResponse{result.AccessToken, "Bearer", result.ExpiresIn, result.User})
}
func (h *Auth) Register(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.service.Register(r.Context(), input.Email, input.Password)
	if err != nil {
		status := http.StatusConflict
		if errors.Is(err, auth.ErrInvalidInput) {
			status = http.StatusBadRequest
		}
		writeAuthError(w, status, "authentication_rejected", "Authentication request rejected.")
		return
	}
	h.respond(w, http.StatusCreated, result)
}
func (h *Auth) Login(w http.ResponseWriter, r *http.Request) {
	var input credentials
	if !decodeJSON(w, r, &input) {
		return
	}
	result, err := h.service.Login(r.Context(), input.Email, input.Password)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return
	}
	h.respond(w, http.StatusOK, result)
}
func (h *Auth) Refresh(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("face_search_refresh")
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return
	}
	result, err := h.service.Refresh(r.Context(), cookie.Value)
	if err != nil {
		h.setCookie(w, "", -1)
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return
	}
	h.respond(w, http.StatusOK, result)
}
func (h *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("face_search_refresh"); err == nil {
		_ = h.service.Logout(r.Context(), cookie.Value)
	}
	h.setCookie(w, "", -1)
	w.WriteHeader(http.StatusNoContent)
}
func (h *Auth) Me(w http.ResponseWriter, r *http.Request) {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return
	}
	user, err := h.service.Me(r.Context(), strings.TrimPrefix(header, "Bearer "))
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return
	}
	writeAuthJSON(w, http.StatusOK, user)
}
