package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/auth"
)

type authTestRepository struct {
	user         auth.User
	passwordHash string
	refreshHash  string
}

func (r *authTestRepository) CreateUserWithSession(_ context.Context, email, passwordHash, refreshHash string, _ time.Time) (auth.User, error) {
	r.user = auth.User{ID: "user-1", Email: email, Status: "active", CreatedAt: time.Unix(1, 0)}
	r.passwordHash, r.refreshHash = passwordHash, refreshHash
	return r.user, nil
}
func (r *authTestRepository) FindUserByEmail(_ context.Context, email string) (auth.User, string, error) {
	if email != r.user.Email {
		return auth.User{}, "", auth.ErrInvalidCredentials
	}
	return r.user, r.passwordHash, nil
}
func (r *authTestRepository) FindUserByID(_ context.Context, id string) (auth.User, error) {
	if id != r.user.ID {
		return auth.User{}, auth.ErrInvalidCredentials
	}
	return r.user, nil
}
func (r *authTestRepository) CreateSession(_ context.Context, userID, hash string, expires time.Time) (auth.Session, error) {
	r.refreshHash = hash
	return auth.Session{ID: "session-1", UserID: userID, Status: "active", ExpiresAt: expires}, nil
}
func (r *authTestRepository) RotateSession(_ context.Context, oldHash, newHash string, expires time.Time) (auth.Session, error) {
	if oldHash != r.refreshHash {
		return auth.Session{}, auth.ErrInvalidCredentials
	}
	r.refreshHash = newHash
	return auth.Session{ID: "session-2", UserID: r.user.ID, Status: "active", ExpiresAt: expires}, nil
}
func (r *authTestRepository) RevokeSession(_ context.Context, hash string) error { return nil }

func newAuthHandler(t *testing.T, secure bool) (*Auth, *authTestRepository) {
	t.Helper()
	repo := &authTestRepository{}
	service, err := auth.NewService(repo, strings.Repeat("s", 32), "issuer", "audience", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	return NewAuth(service, secure, 24*time.Hour), repo
}

func TestAuthRegisterSetsSecureHttpOnlyRefreshCookie(t *testing.T) {
	handler, _ := newAuthHandler(t, true)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct-horse-battery"}`))
	response := httptest.NewRecorder()
	handler.Register(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	cookie := cookies[0]
	if cookie.Name != "face_search_refresh" || !cookie.HttpOnly || !cookie.Secure || cookie.SameSite != http.SameSiteLaxMode || cookie.Path != "/api/v1/auth" || cookie.Value == "" {
		t.Fatalf("unsafe refresh cookie: %#v", cookie)
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["accessToken"] == "" || body["tokenType"] != "Bearer" {
		t.Fatalf("invalid auth response: %v", body)
	}
	if strings.Contains(response.Body.String(), "correct-horse-battery") || strings.Contains(response.Body.String(), cookie.Value) {
		t.Fatal("response leaked credential material")
	}
}

func TestAuthRegisterRejectsUnknownAndTrailingJSON(t *testing.T) {
	handler, _ := newAuthHandler(t, false)
	for _, body := range []string{
		`{"email":"person@example.com","password":"correct-horse-battery","role":"admin"}`,
		`{"email":"person@example.com","password":"correct-horse-battery"} {}`,
	} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
		response := httptest.NewRecorder()
		handler.Register(response, request)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("body=%q status=%d", body, response.Code)
		}
	}
}

func TestAuthLoginUsesGenericFailure(t *testing.T) {
	handler, _ := newAuthHandler(t, false)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"missing@example.com","password":"incorrect-password"}`))
	response := httptest.NewRecorder()
	handler.Login(response, request)
	if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"code\":\"authentication_rejected\",\"message\":\"Authentication request rejected.\"}\n" {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}

func TestAuthRefreshRotatesCookieAndRejectsReplay(t *testing.T) {
	handler, _ := newAuthHandler(t, false)
	registerRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"person@example.com","password":"correct-horse-battery"}`))
	registerResponse := httptest.NewRecorder()
	handler.Register(registerResponse, registerRequest)
	original := registerResponse.Result().Cookies()[0]

	refreshRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	refreshRequest.AddCookie(original)
	refreshResponse := httptest.NewRecorder()
	handler.Refresh(refreshResponse, refreshRequest)
	if refreshResponse.Code != http.StatusOK {
		t.Fatalf("refresh status=%d", refreshResponse.Code)
	}
	replacement := refreshResponse.Result().Cookies()[0]
	if replacement.Value == original.Value {
		t.Fatal("refresh cookie was not rotated")
	}

	replayRequest := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	replayRequest.AddCookie(original)
	replayResponse := httptest.NewRecorder()
	handler.Refresh(replayResponse, replayRequest)
	if replayResponse.Code != http.StatusUnauthorized || replayResponse.Result().Cookies()[0].MaxAge >= 0 {
		t.Fatal("replay was not rejected and cleared")
	}
}

func TestAuthMeRequiresExactBearerScheme(t *testing.T) {
	handler, _ := newAuthHandler(t, false)
	for _, header := range []string{"", "Basic value", "Bearer ", "Bearer  value", "bearer value"} {
		request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		handler.Me(response, request)
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("header=%q status=%d", header, response.Code)
		}
	}
}

func TestAuthLogoutClearsScopedCookie(t *testing.T) {
	handler, _ := newAuthHandler(t, true)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/logout", nil)
	response := httptest.NewRecorder()
	handler.Logout(response, request)
	cookie := response.Result().Cookies()[0]
	if response.Code != http.StatusNoContent || cookie.MaxAge >= 0 || cookie.Value != "" || !cookie.HttpOnly || !cookie.Secure || cookie.Path != "/api/v1/auth" {
		t.Fatalf("logout status=%d cookie=%#v", response.Code, cookie)
	}
}
