package middleware

import (
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/face-search-ai/api/internal/observability"
	"github.com/face-search-ai/api/internal/ratelimit"
)

// SecurityHeaders sets conservative response headers for the JSON API. The API
// never returns HTML, so a full Content-Security-Policy is unnecessary here;
// browser document policy is owned by the web app and the edge proxy. These
// headers defend API responses against MIME sniffing, framing, referrer leakage,
// cross-origin embedding, and incidental caching of sensitive responses.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("Cross-Origin-Resource-Policy", "same-site")
		header.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// KeyFunc derives the rate-limit bucket key for a request. Keys are opaque to the
// limiter; callers combine coarse identifiers (client address, public token) so a
// single abusive source cannot exhaust another's budget.
type KeyFunc func(*http.Request) string

// RateLimit rejects requests that exceed the limiter's budget for their key with a
// safe 429 that never exposes internal detail. A nil or disabled limiter passes
// every request through, keeping the middleware safe to compose unconditionally.
// The surface is a bounded, low-cardinality label (for example "auth" or
// "search") used only to attribute the rejection metric; it is never derived from
// request content.
func RateLimit(limiter *ratelimit.Limiter, key KeyFunc, surface string, next http.Handler) http.Handler {
	if limiter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow(key(r)) {
			observability.RecordRateLimitRejection(surface)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte("{\"code\":\"rate_limited\",\"message\":\"Too many requests. Please try again shortly.\"}\n"))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ClientIPKey keys a limiter by best-effort client address alone.
func ClientIPKey(r *http.Request) string { return ClientIP(r) }

// Timeout bounds handler execution and emits a safe JSON error when a request
// exceeds the deadline instead of leaving the connection to be killed abruptly by
// the server write timeout. A non-positive duration disables the wrapper. The
// underlying handler is buffered, which is safe because every API response is a
// small JSON document rather than a stream.
func Timeout(d time.Duration, next http.Handler) http.Handler {
	if d <= 0 {
		return next
	}
	const body = "{\"code\":\"request_timeout\",\"message\":\"The request timed out. Please try again.\"}\n"
	return http.TimeoutHandler(next, d, body)
}

// ClientIP returns a best-effort client address for coarse abuse control. It
// trusts the leftmost X-Forwarded-For entry set by the reverse proxy and falls
// back to the transport remote address. It is never used for authorization.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if first, _, found := strings.Cut(forwarded, ","); found || first != "" {
			if ip := strings.TrimSpace(first); ip != "" {
				return ip
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
