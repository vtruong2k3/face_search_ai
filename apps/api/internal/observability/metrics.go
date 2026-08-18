// Package observability defines the API's privacy-safe Prometheus metrics and the
// helpers that record them. Every metric label is deliberately bounded and
// low-cardinality: method, a normalized route template, status class, a fixed set
// of operation/outcome/decision tokens, and the fixed set of dependency names.
//
// No label ever carries a raw identifier (user, organization, event, photo, or
// public token), a signed URL, an object path, a credential, an embedding, or any
// biometric or personal data. Correlation identifiers belong in structured logs,
// never in metric labels, because they are unbounded and would explode series
// cardinality while leaking sensitive data.
//
// Metrics register on the default Prometheus registry, which is what the /metrics
// endpoint (promhttp.Handler) serves. The /metrics surface is internal only and is
// never routed through the public reverse proxy.
package observability

import (
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	httpRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests handled, labeled by method, normalized route template, and status class.",
	}, []string{"method", "route", "status_class"})

	httpRequestDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request handling latency in seconds, labeled by method, normalized route template, and status class.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route", "status_class"})

	uploadOperationsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "upload_operations_total",
		Help: "Photo upload lifecycle operations that succeeded, labeled by the bounded operation name.",
	}, []string{"operation"})

	searchRequestsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "search_requests_total",
		Help: "Public selfie-search requests, labeled by a bounded outcome class. Never labeled by token, selfie, or embedding.",
	}, []string{"outcome"})

	searchRequestDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "search_request_duration_seconds",
		Help:    "Public selfie-search end-to-end latency in seconds.",
		Buckets: prometheus.DefBuckets,
	})

	downloadDecisionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "download_decisions_total",
		Help: "Controlled-download decisions, labeled by decision class and request kind. Never labeled by token, URL, or object path.",
	}, []string{"decision", "kind"})

	rateLimitRejectionsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "rate_limit_rejections_total",
		Help: "Requests rejected by a rate limiter, labeled by the bounded protected surface.",
	}, []string{"surface"})

	dependencyUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "dependency_up",
		Help: "Readiness of each downstream dependency (1 healthy, 0 unhealthy), labeled by the fixed dependency name.",
	}, []string{"dependency"})

	dependencyCheckDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "dependency_check_duration_seconds",
		Help:    "Dependency readiness-probe latency in seconds, labeled by dependency name and healthy/unhealthy result.",
		Buckets: prometheus.DefBuckets,
	}, []string{"dependency", "result"})
)

// RecordHTTPRequest records the count and latency of a completed HTTP request.
// The route must already be a bounded template (see NormalizeRoute); status is
// reduced to a class so individual codes do not multiply series.
func RecordHTTPRequest(method, route string, status int, duration time.Duration) {
	class := statusClass(status)
	httpRequestsTotal.WithLabelValues(method, route, class).Inc()
	httpRequestDuration.WithLabelValues(method, route, class).Observe(duration.Seconds())
}

// RecordUploadOperation records one successful upload-lifecycle operation.
func RecordUploadOperation(operation string) {
	uploadOperationsTotal.WithLabelValues(operation).Inc()
}

// RecordSearch records a public selfie-search outcome and its latency. The
// outcome is a bounded class string; the selfie bytes and any embedding are never
// observed.
func RecordSearch(outcome string, duration time.Duration) {
	searchRequestsTotal.WithLabelValues(outcome).Inc()
	searchRequestDuration.Observe(duration.Seconds())
}

// RecordDownloadDecision records one controlled-download decision.
func RecordDownloadDecision(decision, kind string) {
	downloadDecisionsTotal.WithLabelValues(decision, kind).Inc()
}

// RecordRateLimitRejection records a single rate-limit rejection for a surface.
func RecordRateLimitRejection(surface string) {
	rateLimitRejectionsTotal.WithLabelValues(surface).Inc()
}

// RecordDependencyCheck records the outcome and latency of a dependency readiness
// probe. Only the fixed dependency name and a healthy/unhealthy result are
// recorded; the raw error (which could contain a connection string or URL) is
// never used as a label or metric value.
func RecordDependencyCheck(dependency string, healthy bool, duration time.Duration) {
	value := 0.0
	result := "unhealthy"
	if healthy {
		value = 1.0
		result = "healthy"
	}
	dependencyUp.WithLabelValues(dependency).Set(value)
	dependencyCheckDuration.WithLabelValues(dependency, result).Observe(duration.Seconds())
}

// NormalizeRoute reduces a Go 1.22+ ServeMux pattern (for example
// "POST /api/v1/organizations/{organizationId}/events/{eventId}") to a bounded
// route template label. It strips the leading method token because method is a
// separate label, and returns "unmatched" for requests that matched no route so
// unknown paths cannot inflate cardinality.
func NormalizeRoute(pattern string) string {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "unmatched"
	}
	// Patterns are "[METHOD ][HOST]/path". Drop an optional leading method token.
	if space := strings.IndexByte(pattern, ' '); space >= 0 {
		pattern = pattern[space+1:]
	}
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return "unmatched"
	}
	return pattern
}

func statusClass(status int) string {
	switch {
	case status >= 200 && status < 300:
		return "2xx"
	case status >= 300 && status < 400:
		return "3xx"
	case status >= 400 && status < 500:
		return "4xx"
	case status >= 500 && status < 600:
		return "5xx"
	default:
		return strconv.Itoa(status)
	}
}
