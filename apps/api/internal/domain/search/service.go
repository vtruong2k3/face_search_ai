package search

import (
	"context"
	"errors"
	"math"
	"sort"
	"time"

	"github.com/face-search-ai/api/internal/domain/event"
)

var (
	ErrUnavailable = errors.New("search unavailable")
	ErrNotReady    = errors.New("search not ready")
)

func clearBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

type InferredFace struct {
	Embedding []float32
}

type Inference interface {
	ExtractFaces(context.Context, string, []byte) ([]InferredFace, error)
}

type VectorMatch struct {
	PhotoID string
	Score   float32
}

type VectorIndex interface {
	Search(context.Context, string, string, []float32, int) ([]VectorMatch, error)
}

type ScopeResolver interface {
	FindPublicSearchScope(context.Context, string, time.Time) (event.PublicSearchScope, error)
}

type Service struct {
	scopes     ScopeResolver
	inference  Inference
	vectors    VectorIndex
	threshold  float32
	maxResults int
}

func NewService(scopes ScopeResolver, inference Inference, vectors VectorIndex, threshold float32, maxResults int) (*Service, error) {
	if scopes == nil || inference == nil || vectors == nil || threshold < -1 || threshold > 1 || maxResults <= 0 || maxResults > MaxResults {
		return nil, ErrInvalidRequest
	}
	return &Service{scopes: scopes, inference: inference, vectors: vectors, threshold: threshold, maxResults: maxResults}, nil
}

func (s *Service) Search(ctx context.Context, token, contentType string, selfie []byte, consent, consentVersion string) ([]Result, error) {
	defer clearBytes(selfie)
	if err := ValidateRequest(Request{ContentType: contentType, SelfieBytes: int64(len(selfie)), Consent: consent, ConsentVersion: consentVersion, Limit: s.maxResults}); err != nil {
		return nil, err
	}
	scope, err := s.scopes.FindPublicSearchScope(ctx, token, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	faces, err := s.inference.ExtractFaces(ctx, contentType, selfie)
	if err != nil {
		return nil, ErrUnavailable
	}
	if err := RequireExactlyOneFace(len(faces)); err != nil {
		return nil, err
	}
	if len(faces[0].Embedding) == 0 {
		return nil, PolicyError{Code: CodeInvalidImage}
	}
	matches, err := s.vectors.Search(ctx, scope.OrganizationID, scope.EventID, faces[0].Embedding, s.maxResults)
	if err != nil {
		return nil, ErrUnavailable
	}
	return RankPublicMatches(matches, s.threshold, s.maxResults), nil
}

func RankPublicMatches(matches []VectorMatch, threshold float32, maxResults int) []Result {
	best := make(map[string]float32)
	for _, match := range matches {
		if match.PhotoID == "" || math.IsNaN(float64(match.Score)) || math.IsInf(float64(match.Score), 0) || match.Score < threshold {
			continue
		}
		if previous, ok := best[match.PhotoID]; !ok || match.Score > previous {
			best[match.PhotoID] = match.Score
		}
	}
	type ranked struct {
		photoID string
		score   float32
	}
	rankedMatches := make([]ranked, 0, len(best))
	for photoID, score := range best {
		rankedMatches = append(rankedMatches, ranked{photoID: photoID, score: score})
	}
	sort.Slice(rankedMatches, func(i, j int) bool {
		if rankedMatches[i].score == rankedMatches[j].score {
			return rankedMatches[i].photoID < rankedMatches[j].photoID
		}
		return rankedMatches[i].score > rankedMatches[j].score
	})
	if maxResults > 0 && len(rankedMatches) > maxResults {
		rankedMatches = rankedMatches[:maxResults]
	}
	results := make([]Result, 0, len(rankedMatches))
	for _, match := range rankedMatches {
		results = append(results, Result{PhotoID: match.photoID})
	}
	return results
}
