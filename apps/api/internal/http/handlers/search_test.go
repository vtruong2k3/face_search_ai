package handlers

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/face-search-ai/api/internal/domain/search"
)

// TestPublicSearchRejectsOversizedSelfie proves the selfie-search abuse guard: a
// selfie larger than the domain cap is rejected with a safe 413 before the request
// ever reaches the (here nil) search service, so no inference is attempted.
func TestPublicSearchRejectsOversizedSelfie(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	_ = writer.WriteField("consent", "true")
	_ = writer.WriteField("consentVersion", "v1")
	part, err := writer.CreateFormFile("selfie", "selfie.jpg")
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	oversized := bytes.Repeat([]byte{0x7f}, search.MaxSelfieBytes+1)
	if _, err := part.Write(oversized); err != nil {
		t.Fatalf("write selfie: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/public/events/token", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	request.SetPathValue("publicToken", "token")

	handler := NewSearch(nil)
	response := httptest.NewRecorder()
	handler.Public(response, request)

	if response.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversized selfie = %d, want 413", response.Code)
	}
	if body := response.Body.String(); body != "{\"code\":\"selfie_too_large\",\"message\":\"The selfie is too large.\"}\n" {
		t.Fatalf("unexpected 413 body: %q", body)
	}
}
