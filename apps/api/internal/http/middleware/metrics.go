package middleware

import (
	"net/http"
	"time"

	"github.com/face-search-ai/api/internal/observability"
)

// statusRecorder captures the response status code so request metrics can be
// labeled by status class. It defaults to 200, matching net/http's behavior when
// a handler writes a body without an explicit WriteHeader.
type statusRecorder struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if !s.wroteHeader {
		s.status = status
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.status = http.StatusOK
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// Metrics records HTTP request count and latency labeled only by bounded
// dimensions (method, normalized route template, status class). It must wrap the
// ServeMux directly so the matched route pattern is available on the request when
// the handler returns; the route template is bounded and never contains request
// values, so it is safe as a metric label.
func Metrics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, r)
		observability.RecordHTTPRequest(r.Method, observability.NormalizeRoute(r.Pattern), recorder.status, time.Since(started))
	})
}
