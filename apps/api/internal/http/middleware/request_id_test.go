package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestIDAcceptsSafeBoundedIdentifier(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set(RequestIDHeader, "client_request-123")
	response := httptest.NewRecorder()
	RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := RequestIDFromContext(r.Context()); got != "client_request-123" {
			t.Fatalf("request ID=%q", got)
		}
		w.WriteHeader(http.StatusNoContent)
	})).ServeHTTP(response, request)
	if response.Header().Get(RequestIDHeader) != "client_request-123" {
		t.Fatalf("response request ID=%q", response.Header().Get(RequestIDHeader))
	}
}

func TestRequestIDReplacesUnsafeIdentifier(t *testing.T) {
	for _, incoming := range []string{"", "contains space", "line\nbreak", strings.Repeat("a", 65)} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set(RequestIDHeader, incoming)
		response := httptest.NewRecorder()
		RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			generated := RequestIDFromContext(r.Context())
			if !validRequestID(generated) || generated == incoming {
				t.Fatalf("incoming=%q generated=%q", incoming, generated)
			}
			w.WriteHeader(http.StatusNoContent)
		})).ServeHTTP(response, request)
		if !validRequestID(response.Header().Get(RequestIDHeader)) {
			t.Fatalf("unsafe response request ID=%q", response.Header().Get(RequestIDHeader))
		}
	}
}
