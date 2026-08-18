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

type FaceAIClient struct {
	baseURL string
	token   string
	http    *http.Client
}

func NewFaceAIClient(baseURL, token string, client *http.Client) (*FaceAIClient, error) {
	if strings.TrimSpace(baseURL) == "" || client == nil {
		return nil, fmt.Errorf("invalid face ai client configuration")
	}
	return &FaceAIClient{baseURL: strings.TrimRight(baseURL, "/"), token: token, http: client}, nil
}

type inferenceResponse struct {
	Faces []struct {
		Embedding []float32 `json:"embedding"`
	} `json:"faces"`
}

func (c *FaceAIClient) ExtractFaces(ctx context.Context, contentType string, image []byte) ([]search.InferredFace, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/internal/v1/extract-faces", bytes.NewReader(image))
	if err != nil {
		return nil, search.ErrUnavailable
	}
	req.Header.Set("Content-Type", contentType)
	if c.token != "" {
		req.Header.Set("X-Internal-Token", c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, search.ErrUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil, search.ErrUnavailable
	}
	var decoded inferenceResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 20<<20)).Decode(&decoded); err != nil {
		return nil, search.ErrUnavailable
	}
	faces := make([]search.InferredFace, 0, len(decoded.Faces))
	for _, face := range decoded.Faces {
		if len(face.Embedding) == 0 {
			return nil, search.ErrUnavailable
		}
		faces = append(faces, search.InferredFace{Embedding: face.Embedding})
	}
	return faces, nil
}
