package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/ratelimit"
)

func TestSecurityHeadersSetsConservativeDefaults(t *testing.T) {
	handler := SecurityHeaders(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil))

	want := map[string]string{
		"X-Content-Type-Options":       "nosniff",
		"X-Frame-Options":              "DENY",
		"Referrer-Policy":              "no-referrer",
		"Cross-Origin-Resource-Policy": "same-site",
		"Cache-Control":                "no-store",
	}
	for header, value := range want {
		if got := response.Header().Get(header); got != value {
			t.Fatalf("%s = %q, want %q", header, got, value)
		}
	}
}

func TestRateLimitAllowsUnderLimitThenReturns429(t *testing.T) {
	handler := RateLimit(ratelimit.New(1, time.Minute), ClientIPKey, "test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if first.Code != http.StatusOK {
		t.Fatalf("first request within budget = %d", first.Code)
	}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request over budget = %d", second.Code)
	}
	if body := second.Body.String(); body != "{\"code\":\"rate_limited\",\"message\":\"Too many requests. Please try again shortly.\"}\n" {
		t.Fatalf("unexpected 429 body: %q", body)
	}
}

func TestRateLimitIsolatesKeys(t *testing.T) {
	handler := RateLimit(ratelimit.New(1, time.Minute), ClientIPKey, "test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	for _, address := range []string{"10.0.0.1:1", "10.0.0.2:1"} {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		request.RemoteAddr = address
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("distinct client %s must have its own budget, got %d", address, response.Code)
		}
	}
}

func TestRateLimitNilLimiterIsPassthrough(t *testing.T) {
	called := 0
	handler := RateLimit(nil, ClientIPKey, "test", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called++
		w.WriteHeader(http.StatusOK)
	}))
	for i := 0; i < 5; i++ {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/", nil))
	}
	if called != 5 {
		t.Fatalf("nil limiter must pass every request, called=%d", called)
	}
}

func TestTimeoutReturns503ForSlowHandler(t *testing.T) {
	handler := Timeout(10*time.Millisecond, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/organizations", nil))
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("slow handler = %d, want 503", response.Code)
	}
	if body := response.Body.String(); body != "{\"code\":\"request_timeout\",\"message\":\"The request timed out. Please try again.\"}\n" {
		t.Fatalf("unexpected timeout body: %q", body)
	}
}

func TestTimeoutPassesFastHandler(t *testing.T) {
	handler := Timeout(time.Second, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusCreated {
		t.Fatalf("fast handler = %d, want 201", response.Code)
	}
}

func TestTimeoutDisabledForNonPositiveDuration(t *testing.T) {
	handler := Timeout(0, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(20 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("disabled timeout must not intercept, got %d", response.Code)
	}
}

func TestClientIPPrefersForwardedForThenRemoteAddr(t *testing.T) {
	forwarded := httptest.NewRequest(http.MethodGet, "/", nil)
	forwarded.RemoteAddr = "10.0.0.9:5000"
	forwarded.Header.Set("X-Forwarded-For", "203.0.113.7, 10.0.0.9")
	if got := ClientIP(forwarded); got != "203.0.113.7" {
		t.Fatalf("forwarded client = %q, want 203.0.113.7", got)
	}

	direct := httptest.NewRequest(http.MethodGet, "/", nil)
	direct.RemoteAddr = "198.51.100.4:6000"
	if got := ClientIP(direct); got != "198.51.100.4" {
		t.Fatalf("direct client = %q, want 198.51.100.4", got)
	}
}
