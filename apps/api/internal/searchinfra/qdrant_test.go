package searchinfra

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/face-search-ai/api/internal/domain/search"
)

// capturedRequest records the exact JSON body Qdrant received so tests can assert
// the tenant and Event filters are always present and correct.
func newCapturingQdrant(t *testing.T, response string, captured *qdrantSearchRequest) *QdrantClient {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, captured); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(server.Close)
	client, err := NewQdrantClient(server.URL, "faces", server.Client())
	if err != nil {
		t.Fatalf("NewQdrantClient() error = %v", err)
	}
	return client
}

func filterValue(conditions []qdrantCondition, key string) (string, bool) {
	for _, condition := range conditions {
		if condition.Key == key {
			return condition.Match.Value, true
		}
	}
	return "", false
}

func TestQdrantSearchAlwaysIncludesOrganizationAndEventFilters(t *testing.T) {
	var captured qdrantSearchRequest
	client := newCapturingQdrant(t, `{"result":[{"score":0.9,"payload":{"photo_id":"photo-1"}}]}`, &captured)

	matches, err := client.Search(context.Background(), "org-1", "event-1", []float32{1, 0, 0}, 10)
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(matches) != 1 || matches[0].PhotoID != "photo-1" {
		t.Fatalf("unexpected matches: %#v", matches)
	}

	organizationID, ok := filterValue(captured.Filter.Must, "organization_id")
	if !ok || organizationID != "org-1" {
		t.Fatalf("organization_id filter missing or wrong: %q (%v)", organizationID, captured.Filter.Must)
	}
	eventID, ok := filterValue(captured.Filter.Must, "event_id")
	if !ok || eventID != "event-1" {
		t.Fatalf("event_id filter missing or wrong: %q (%v)", eventID, captured.Filter.Must)
	}
	if captured.WithVectors {
		t.Fatal("Qdrant request must not request stored vectors back")
	}
}

// Adversarial: a caller must never be able to issue a scope-less query. Empty
// organization or Event scope is rejected before any network call is made.
func TestQdrantSearchRejectsMissingScope(t *testing.T) {
	var captured qdrantSearchRequest
	client := newCapturingQdrant(t, `{"result":[]}`, &captured)

	cases := []struct {
		name           string
		organizationID string
		eventID        string
		vector         []float32
	}{
		{name: "missing organization", organizationID: "", eventID: "event-1", vector: []float32{1}},
		{name: "missing event", organizationID: "org-1", eventID: "", vector: []float32{1}},
		{name: "missing vector", organizationID: "org-1", eventID: "event-1", vector: nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := client.Search(context.Background(), testCase.organizationID, testCase.eventID, testCase.vector, 10)
			if !errors.Is(err, search.ErrUnavailable) {
				t.Fatalf("expected ErrUnavailable, got %v", err)
			}
			if captured.Vector != nil || len(captured.Filter.Must) != 0 {
				t.Fatal("scope-less query must not reach Qdrant")
			}
		})
	}
}
