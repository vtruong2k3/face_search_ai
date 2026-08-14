package http_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	httpserver "github.com/face-search-ai/api/internal/http"
	"github.com/face-search-ai/api/internal/platform"
)

type checker struct{ statuses map[string]platform.Status }

func (c checker) Check(context.Context) map[string]platform.Status { return c.statuses }

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

func TestReadinessFailure(t *testing.T) {
	router := httpserver.NewRouter(checker{statuses: map[string]platform.Status{"redis": {OK: false, Error: "unavailable"}}})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health/ready", nil))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("ready returned %d", recorder.Code)
	}
}
