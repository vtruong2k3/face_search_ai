package searchinfra

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/face-search-ai/api/internal/domain/search"
)

type QdrantClient struct {
	baseURL    string
	collection string
	http       *http.Client
}

func NewQdrantClient(baseURL, collection string, client *http.Client) (*QdrantClient, error) {
	if strings.TrimSpace(baseURL) == "" || strings.TrimSpace(collection) == "" || client == nil {
		return nil, fmt.Errorf("invalid qdrant client configuration")
	}
	return &QdrantClient{baseURL: strings.TrimRight(baseURL, "/"), collection: collection, http: client}, nil
}

type qdrantSearchRequest struct {
	Vector      []float32    `json:"vector"`
	Limit       int          `json:"limit"`
	WithPayload bool         `json:"with_payload"`
	WithVectors bool         `json:"with_vectors"`
	Filter      qdrantFilter `json:"filter"`
}

type qdrantFilter struct {
	Must []qdrantCondition `json:"must"`
}

type qdrantCondition struct {
	Key   string      `json:"key"`
	Match qdrantMatch `json:"match"`
}

type qdrantMatch struct {
	Value string `json:"value"`
}

type qdrantSearchResponse struct {
	Result []struct {
		Score   float32 `json:"score"`
		Payload struct {
			PhotoID string `json:"photo_id"`
		} `json:"payload"`
	} `json:"result"`
}

func (c *QdrantClient) Search(ctx context.Context, organizationID, eventID string, vector []float32, limit int) ([]search.VectorMatch, error) {
	if organizationID == "" || eventID == "" || len(vector) == 0 || limit <= 0 {
		return nil, search.ErrUnavailable
	}
	body := qdrantSearchRequest{
		Vector: vector, Limit: limit, WithPayload: true, WithVectors: false,
		Filter: qdrantFilter{Must: []qdrantCondition{
			{Key: "organization_id", Match: qdrantMatch{Value: organizationID}},
			{Key: "event_id", Match: qdrantMatch{Value: eventID}},
		}},
	}
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, search.ErrUnavailable
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/collections/"+c.collection+"/points/search", bytes.NewReader(encoded))
	if err != nil {
		return nil, search.ErrUnavailable
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, search.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, search.ErrUnavailable
	}
	var decoded qdrantSearchResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&decoded); err != nil {
		return nil, search.ErrUnavailable
	}
	matches := make([]search.VectorMatch, 0, len(decoded.Result))
	for _, result := range decoded.Result {
		matches = append(matches, search.VectorMatch{PhotoID: result.Payload.PhotoID, Score: result.Score})
	}
	return matches, nil
}
