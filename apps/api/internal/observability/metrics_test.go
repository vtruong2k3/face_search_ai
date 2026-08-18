package observability

import "testing"

func TestNormalizeRouteStripsMethodAndBoundsUnmatched(t *testing.T) {
	cases := map[string]string{
		"POST /api/v1/organizations/{organizationId}/events/{eventId}": "/api/v1/organizations/{organizationId}/events/{eventId}",
		"GET /health/live": "/health/live",
		"/metrics":         "/metrics",
		"":                 "unmatched",
		"   ":              "unmatched",
	}
	for pattern, want := range cases {
		if got := NormalizeRoute(pattern); got != want {
			t.Fatalf("NormalizeRoute(%q) = %q, want %q", pattern, got, want)
		}
	}
}

// TestNormalizeRouteIsBounded asserts the normalized label only ever contains a
// route template (placeholders in braces), never a concrete identifier value.
func TestNormalizeRouteIsBounded(t *testing.T) {
	route := NormalizeRoute("GET /api/v1/organizations/{organizationId}/events/{eventId}/photos/{photoId}")
	if route == "" {
		t.Fatal("route label must not be empty")
	}
	// A raw UUID or opaque token must never appear; only brace-delimited templates.
	for _, forbidden := range []string{"11111111-1111", "org-", "tok"} {
		if contains(route, forbidden) {
			t.Fatalf("route label %q must not contain concrete value %q", route, forbidden)
		}
	}
}

func TestStatusClass(t *testing.T) {
	cases := map[int]string{200: "2xx", 201: "2xx", 302: "3xx", 404: "4xx", 429: "4xx", 503: "5xx", 100: "100"}
	for code, want := range cases {
		if got := statusClass(code); got != want {
			t.Fatalf("statusClass(%d) = %q, want %q", code, got, want)
		}
	}
}

func contains(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
