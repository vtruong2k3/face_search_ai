package search

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/event"
)

type fakeScopeResolver struct{ scope event.PublicSearchScope }

func (f fakeScopeResolver) FindPublicSearchScope(context.Context, string, time.Time) (event.PublicSearchScope, error) {
	return f.scope, nil
}

type fakeInference struct{ faces []InferredFace }

func (f fakeInference) ExtractFaces(context.Context, string, []byte) ([]InferredFace, error) {
	return f.faces, nil
}

type fakeVectorIndex struct {
	organizationID, eventID string
	matches                 []VectorMatch
}

func (f *fakeVectorIndex) Search(_ context.Context, organizationID, eventID string, _ []float32, _ int) ([]VectorMatch, error) {
	f.organizationID, f.eventID = organizationID, eventID
	return f.matches, nil
}

func TestRankPublicMatchesDeduplicatesAndSorts(t *testing.T) {
	results := RankPublicMatches([]VectorMatch{
		{PhotoID: "b", Score: .8}, {PhotoID: "a", Score: .8}, {PhotoID: "a", Score: .9}, {PhotoID: "c", Score: .2},
	}, .5, 100)
	if len(results) != 2 || results[0].PhotoID != "a" || results[1].PhotoID != "b" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestSearchUsesTrustedEventAndOrganizationScope(t *testing.T) {
	vectors := &fakeVectorIndex{matches: []VectorMatch{{PhotoID: "photo", Score: .9}}}
	service, err := NewService(fakeScopeResolver{scope: event.PublicSearchScope{OrganizationID: "org-1", EventID: "event-1"}}, fakeInference{faces: []InferredFace{{Embedding: []float32{1, 0}}}}, vectors, .5, 10)
	if err != nil {
		t.Fatal(err)
	}
	results, err := service.Search(context.Background(), "opaque", "image/jpeg", []byte("ephemeral"), "true", "v1")
	if err != nil || len(results) != 1 {
		t.Fatalf("search failed: %v %#v", err, results)
	}
	if vectors.organizationID != "org-1" || vectors.eventID != "event-1" {
		t.Fatalf("scope not applied: %q/%q", vectors.organizationID, vectors.eventID)
	}
}

func TestSearchRejectsMultipleFaces(t *testing.T) {
	service, err := NewService(fakeScopeResolver{}, fakeInference{faces: []InferredFace{{}, {}}}, &fakeVectorIndex{}, .5, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Search(context.Background(), "opaque", "image/jpeg", []byte("ephemeral"), "true", "v1")
	if _, ok := err.(FaceCountError); !ok {
		t.Fatalf("expected face count error, got %T %v", err, err)
	}
}

func TestSearchRejectsZeroFaces(t *testing.T) {
	service, err := NewService(fakeScopeResolver{}, fakeInference{faces: nil}, &fakeVectorIndex{}, .5, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Search(context.Background(), "opaque", "image/jpeg", []byte("ephemeral"), "true", "v1")
	faceError, ok := err.(FaceCountError)
	if !ok || faceError.Count != 0 {
		t.Fatalf("expected zero-face error, got %T %v", err, err)
	}
}

// RankPublicMatches must drop matches below the (non-production) threshold and
// discard NaN/Inf scores that could otherwise leak through ordering.
func TestRankPublicMatchesFiltersBelowThresholdAndInvalidScores(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	results := RankPublicMatches([]VectorMatch{
		{PhotoID: "keep", Score: .6},
		{PhotoID: "drop-low", Score: .49},
		{PhotoID: "drop-nan", Score: nan},
		{PhotoID: "drop-inf", Score: inf},
		{PhotoID: "", Score: .99},
	}, .5, 100)
	if len(results) != 1 || results[0].PhotoID != "keep" {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestRankPublicMatchesHonorsMaxResults(t *testing.T) {
	results := RankPublicMatches([]VectorMatch{
		{PhotoID: "a", Score: .9}, {PhotoID: "b", Score: .8}, {PhotoID: "c", Score: .7},
	}, .5, 2)
	if len(results) != 2 || results[0].PhotoID != "a" || results[1].PhotoID != "b" {
		t.Fatalf("maxResults not honored: %#v", results)
	}
}

// Selfie bytes must not survive the request. The service zeroes the buffer before
// returning so no plaintext biometric input remains in post-request state.
func TestSearchClearsSelfieBytes(t *testing.T) {
	vectors := &fakeVectorIndex{matches: []VectorMatch{{PhotoID: "photo", Score: .9}}}
	service, err := NewService(fakeScopeResolver{scope: event.PublicSearchScope{OrganizationID: "org-1", EventID: "event-1"}}, fakeInference{faces: []InferredFace{{Embedding: []float32{1, 0}}}}, vectors, .5, 10)
	if err != nil {
		t.Fatal(err)
	}
	selfie := []byte("ephemeral-selfie-bytes")
	if _, err := service.Search(context.Background(), "opaque", "image/jpeg", selfie, "true", "v1"); err != nil {
		t.Fatalf("search failed: %v", err)
	}
	for index, value := range selfie {
		if value != 0 {
			t.Fatalf("selfie byte %d not cleared: %d", index, value)
		}
	}
}
