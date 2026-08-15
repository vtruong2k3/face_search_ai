package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSAllowsOnlyConfiguredCredentialedOrigin(t *testing.T) {
	nextCalled := false
	handler := CORS("http://localhost:3000", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	}))

	request := httptest.NewRequest(http.MethodGet, "/api/v1/auth/me", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK || !nextCalled {
		t.Fatalf("allowed origin status=%d called=%v", response.Code, nextCalled)
	}
	if response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" || response.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Fatalf("credentialed CORS headers = %v", response.Header())
	}
}

func TestCORSRejectsForeignOriginBeforeHandler(t *testing.T) {
	nextCalled := false
	handler := CORS("http://localhost:3000", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { nextCalled = true }))
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	request.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusForbidden || nextCalled {
		t.Fatalf("foreign origin status=%d called=%v", response.Code, nextCalled)
	}
}

func TestCORSHandlesAllowedPreflight(t *testing.T) {
	handler := CORS("http://localhost:3000", http.HandlerFunc(func(http.ResponseWriter, *http.Request) { t.Fatal("preflight reached application handler") }))
	request := httptest.NewRequest(http.MethodOptions, "/api/v1/auth/login", nil)
	request.Header.Set("Origin", "http://localhost:3000")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Fatalf("preflight status=%d headers=%v", response.Code, response.Header())
	}
}
