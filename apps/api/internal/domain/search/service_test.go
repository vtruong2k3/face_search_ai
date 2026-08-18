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

// fakeVisibility records the scope it was queried with and returns only the
// photo IDs in its allow-set, modeling the DB tombstone/READY enforcement.
type fakeVisibility struct {
	organizationID, eventID string
	allow                   map[string]struct{}
	err                     error
}

func passthroughVisibility() *fakeVisibility { return &fakeVisibility{} }

func (f *fakeVisibility) FilterVisiblePhotoIDs(_ context.Context, organizationID, eventID string, photoIDs []string) ([]string, error) {
	f.organizationID, f.eventID = organizationID, eventID
	if f.err != nil {
		return nil, f.err
	}
	if f.allow == nil {
		return photoIDs, nil
	}
	out := make([]string, 0, len(photoIDs))
	for _, id := range photoIDs {
		if _, ok := f.allow[id]; ok {
			out = append(out, id)
		}
	}
	return out, nil
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
	visibility := passthroughVisibility()
	service, err := NewService(fakeScopeResolver{scope: event.PublicSearchScope{OrganizationID: "org-1", EventID: "event-1"}}, fakeInference{faces: []InferredFace{{Embedding: []float32{1, 0}}}}, vectors, visibility, .5, 10)
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
	if visibility.organizationID != "org-1" || visibility.eventID != "event-1" {
		t.Fatalf("visibility scope not applied: %q/%q", visibility.organizationID, visibility.eventID)
	}
}

// A photo that is tombstoned/deleted (or whose Event was archived) must not
// appear in results even if its vectors are still present in the index. The
// visibility filter, backed by DB state, removes it.
func TestSearchExcludesTombstonedPhotos(t *testing.T) {
	vectors := &fakeVectorIndex{matches: []VectorMatch{{PhotoID: "kept", Score: .9}, {PhotoID: "deleted", Score: .95}}}
	visibility := &fakeVisibility{allow: map[string]struct{}{"kept": {}}}
	service, err := NewService(fakeScopeResolver{scope: event.PublicSearchScope{OrganizationID: "org-1", EventID: "event-1"}}, fakeInference{faces: []InferredFace{{Embedding: []float32{1, 0}}}}, vectors, visibility, .5, 10)
	if err != nil {
		t.Fatal(err)
	}
	results, err := service.Search(context.Background(), "opaque", "image/jpeg", []byte("ephemeral"), "true", "v1")
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if len(results) != 1 || results[0].PhotoID != "kept" {
		t.Fatalf("expected only visible photo, got %#v", results)
	}
}

// The visibility filter fails closed: a lookup error surfaces as ErrUnavailable
// rather than returning unfiltered (possibly deleted) results.
func TestSearchFailsClosedWhenVisibilityErrors(t *testing.T) {
	vectors := &fakeVectorIndex{matches: []VectorMatch{{PhotoID: "photo", Score: .9}}}
	visibility := &fakeVisibility{err: ErrUnavailable}
	service, err := NewService(fakeScopeResolver{scope: event.PublicSearchScope{OrganizationID: "org-1", EventID: "event-1"}}, fakeInference{faces: []InferredFace{{Embedding: []float32{1, 0}}}}, vectors, visibility, .5, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Search(context.Background(), "opaque", "image/jpeg", []byte("ephemeral"), "true", "v1"); err != ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestSearchRejectsMultipleFaces(t *testing.T) {
	service, err := NewService(fakeScopeResolver{}, fakeInference{faces: []InferredFace{{}, {}}}, &fakeVectorIndex{}, passthroughVisibility(), .5, 10)
	if err != nil {
		t.Fatal(err)
	}
	_, err = service.Search(context.Background(), "opaque", "image/jpeg", []byte("ephemeral"), "true", "v1")
	if _, ok := err.(FaceCountError); !ok {
		t.Fatalf("expected face count error, got %T %v", err, err)
	}
}

func TestSearchRejectsZeroFaces(t *testing.T) {
	service, err := NewService(fakeScopeResolver{}, fakeInference{faces: nil}, &fakeVectorIndex{}, passthroughVisibility(), .5, 10)
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
	service, err := NewService(fakeScopeResolver{scope: event.PublicSearchScope{OrganizationID: "org-1", EventID: "event-1"}}, fakeInference{faces: []InferredFace{{Embedding: []float32{1, 0}}}}, vectors, passthroughVisibility(), .5, 10)
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
