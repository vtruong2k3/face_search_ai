package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/auth"
	httpserver "github.com/face-search-ai/api/internal/http"
	"github.com/face-search-ai/api/internal/http/handlers"
	"github.com/face-search-ai/api/internal/platform"
	"github.com/face-search-ai/api/internal/ratelimit"
)

type checker struct{ statuses map[string]platform.Status }

func (c checker) Check(context.Context) map[string]platform.Status { return c.statuses }

// recordingChecker fails the test if its dependency check is ever invoked. It
// proves liveness reflects only process health and never probes dependencies.
type recordingChecker struct {
	t      *testing.T
	called bool
}

func (c *recordingChecker) Check(context.Context) map[string]platform.Status {
	c.called = true
	c.t.Fatal("liveness must not invoke dependency checks")
	return nil
}

// rejectingAuthRepository fails every lookup so login always returns a generic
// authentication failure; it lets the router-level rate-limit test drive the
// login route without a database.
type rejectingAuthRepository struct{}

func (rejectingAuthRepository) CreateUserWithSession(context.Context, string, string, string, time.Time) (auth.User, error) {
	return auth.User{}, auth.ErrInvalidCredentials
}
func (rejectingAuthRepository) FindUserByEmail(context.Context, string) (auth.User, string, error) {
	return auth.User{}, "", auth.ErrInvalidCredentials
}
func (rejectingAuthRepository) FindUserByID(context.Context, string) (auth.User, error) {
	return auth.User{}, auth.ErrInvalidCredentials
}
func (rejectingAuthRepository) CreateSession(context.Context, string, string, time.Time) (auth.Session, error) {
	return auth.Session{}, auth.ErrInvalidCredentials
}
func (rejectingAuthRepository) RotateSession(context.Context, string, string, time.Time) (auth.Session, error) {
	return auth.Session{}, auth.ErrInvalidCredentials
}
func (rejectingAuthRepository) RevokeSession(context.Context, string) error { return nil }

func TestHealthEndpoints(t *testing.T) {
	router := httpserver.NewRouter(checker{statuses: map[string]platform.Status{"redis": {OK: true}}})
	for _, path := range []string{"/health/live", "/health/ready", "/metrics"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		router.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, recorder.Code)
		}
	}
}

func TestLivenessDoesNotProbeDependencies(t *testing.T) {
	// Liveness must be independent of dependency readiness: a checker that would
	// panic on use proves /health/live never calls it.
	router := httpserver.NewRouter(&recordingChecker{t: t})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("liveness returned %d", recorder.Code)
	}
}

func TestMetricsEndpointExposesBoundedRequestSeries(t *testing.T) {
	router := httpserver.NewRouter(checker{statuses: map[string]platform.Status{"redis": {OK: true}}})

	// Drive a request through a known route so its bounded series is recorded.
	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/health/live", nil))

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("metrics returned %d", recorder.Code)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "http_requests_total") {
		t.Fatalf("metrics must expose http_requests_total, got:\n%s", body)
	}
	// The route label must be a bounded template/path, and status a class, not a raw code.
	if !strings.Contains(body, `route="/health/live"`) {
		t.Fatalf("metrics must label the bounded route template, got:\n%s", body)
	}
	if !strings.Contains(body, `status_class="2xx"`) {
		t.Fatalf("metrics must label a bounded status class, got:\n%s", body)
	}
}

func TestReadinessFailure(t *testing.T) {
	router := httpserver.NewRouter(checker{statuses: map[string]platform.Status{"redis": {OK: false, Error: "unavailable"}}})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready returned %d", recorder.Code)
	}
}

func TestRouterAppliesSecurityHeadersAndRequestID(t *testing.T) {
	router := httpserver.NewRouter(checker{statuses: map[string]platform.Status{"redis": {OK: true}}})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	if recorder.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("missing nosniff header: %v", recorder.Header())
	}
	if recorder.Header().Get("X-Request-ID") == "" {
		t.Fatal("responses must carry a correlation request ID")
	}
}

func TestRouterCORSPreflightAdvertisesStateChangingMethods(t *testing.T) {
	router := httpserver.NewRouterWithAuth(checker{}, nil, nil, nil, nil, nil, nil, nil, httpserver.SecurityControls{WebOrigin: "http://localhost:3000"})
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/organizations/org/events/evt", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("preflight status=%d", recorder.Code)
	}
	methods := recorder.Header().Get("Access-Control-Allow-Methods")
	for _, method := range []string{"PATCH", "DELETE"} {
		if !strings.Contains(methods, method) {
			t.Fatalf("preflight must advertise %s, got %q", method, methods)
		}
	}
}

func TestRouterRateLimitsPublicSearch(t *testing.T) {
	searchHandler := handlers.NewSearch(nil)
	router := httpserver.NewRouterWithAuth(checker{}, nil, nil, nil, nil, nil, searchHandler, nil, httpserver.SecurityControls{
		SearchLimiter: ratelimit.New(1, time.Minute),
	})

	// The first request passes the limiter and reaches the handler (rejected as a
	// malformed multipart, not throttled); the second is throttled with 429.
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/public/events/tok", strings.NewReader("{}")))
	if first.Code == http.StatusTooManyRequests {
		t.Fatalf("first search request must not be throttled, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/public/events/tok", strings.NewReader("{}")))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second search request must be throttled, got %d", second.Code)
	}
}

func TestRouterRateLimitsAuthLogin(t *testing.T) {
	service, err := auth.NewService(rejectingAuthRepository{}, strings.Repeat("s", 32), "issuer", "audience", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatalf("auth service: %v", err)
	}
	authHandler := handlers.NewAuth(service, false, 24*time.Hour)
	router := httpserver.NewRouterWithAuth(checker{}, authHandler, service, nil, nil, nil, nil, nil, httpserver.SecurityControls{
		AuthLimiter: ratelimit.New(1, time.Minute),
	})

	body := `{"email":"person@example.com","password":"incorrect-password"}`
	first := httptest.NewRecorder()
	router.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body)))
	if first.Code != http.StatusUnauthorized {
		t.Fatalf("first login should reach handler and fail auth, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	router.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body)))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second login should be throttled, got %d", second.Code)
	}
}
