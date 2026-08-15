package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/domain/authorization"
)

type authenticateRepository struct {
	user auth.User
}

func (r *authenticateRepository) CreateUserWithSession(_ context.Context, email, _, _ string, _ time.Time) (auth.User, error) {
	r.user = auth.User{ID: "trusted-user", Email: email, Status: "active", CreatedAt: time.Unix(1, 0)}
	return r.user, nil
}
func (r *authenticateRepository) FindUserByEmail(context.Context, string) (auth.User, string, error) {
	return auth.User{}, "", auth.ErrInvalidCredentials
}
func (r *authenticateRepository) FindUserByID(context.Context, string) (auth.User, error) {
	return r.user, nil
}
func (r *authenticateRepository) CreateSession(context.Context, string, string, time.Time) (auth.Session, error) {
	return auth.Session{}, nil
}
func (r *authenticateRepository) RotateSession(context.Context, string, string, time.Time) (auth.Session, error) {
	return auth.Session{}, auth.ErrInvalidCredentials
}
func (r *authenticateRepository) RevokeSession(context.Context, string) error { return nil }

func newAuthenticateService(t *testing.T) (*auth.Service, string) {
	t.Helper()
	service, err := auth.NewService(&authenticateRepository{}, strings.Repeat("s", 32), "issuer", "audience", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	result, err := service.Register(context.Background(), "person@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	return service, result.AccessToken
}

func TestAuthenticateRejectsMalformedCredentialsGenerically(t *testing.T) {
	service, _ := newAuthenticateService(t)
	for _, header := range []string{"", "Basic value", "bearer value", "Bearer ", "Bearer two parts", "Bearer invalid"} {
		request := httptest.NewRequest(http.MethodGet, "/protected", nil)
		request.Header.Set("Authorization", header)
		response := httptest.NewRecorder()
		Authenticate(service, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
			t.Fatal("rejected request reached protected handler")
		})).ServeHTTP(response, request)
		if response.Code != http.StatusUnauthorized || response.Body.String() != "{\"code\":\"authentication_rejected\",\"message\":\"Authentication request rejected.\"}\n" {
			t.Fatalf("header=%q status=%d body=%q", header, response.Code, response.Body.String())
		}
	}
}

func TestAuthenticatePropagatesOnlyTrustedPrincipal(t *testing.T) {
	service, token := newAuthenticateService(t)
	request := httptest.NewRequest(http.MethodGet, "/protected?userId=attacker&role=owner", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	Authenticate(service, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		principal, ok := authorization.PrincipalFromContext(r.Context())
		if !ok || principal.UserID != "trusted-user" {
			t.Fatalf("principal=%#v ok=%v", principal, ok)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
