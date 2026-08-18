package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/domain/download"
	"github.com/face-search-ai/api/internal/ratelimit"
)

const readyPhoto = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"

type stubScopeResolver struct {
	scope download.Scope
	err   error
}

func (s stubScopeResolver) FindPublicDownloadScope(context.Context, string, time.Time) (download.Scope, error) {
	return s.scope, s.err
}

type stubPhotoResolver struct {
	objects map[string]download.DownloadableObject
}

func (s stubPhotoResolver) FindDownloadable(_ context.Context, _, _, photoID string) (download.DownloadableObject, error) {
	object, ok := s.objects[photoID]
	if !ok {
		return download.DownloadableObject{}, errors.New("not found")
	}
	return object, nil
}

type stubSigner struct{}

func (stubSigner) PresignGet(_ context.Context, objectKey, _ string, ttl time.Duration) (string, time.Time, error) {
	return "https://minio.example/" + objectKey, time.Now().Add(ttl), nil
}

type stubRecorder struct{}

func (stubRecorder) Record(context.Context, download.AuditEntry) error { return nil }

type capturingAuditor struct{ records []authorization.AuditRecord }

func (c *capturingAuditor) WriteAudit(_ context.Context, record authorization.AuditRecord) error {
	c.records = append(c.records, record)
	return nil
}

func newDownloadsHandler(t *testing.T, scope download.Scope, scopeErr error, limit int) (*Downloads, *capturingAuditor) {
	t.Helper()
	service, err := download.NewService(
		stubScopeResolver{scope: scope, err: scopeErr},
		stubPhotoResolver{objects: map[string]download.DownloadableObject{readyPhoto: {ObjectKey: "organizations/o/events/e/photos/p/original", ContentType: "image/jpeg"}}},
		stubSigner{},
		stubRecorder{},
		time.Minute,
		50,
	)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	auditor := &capturingAuditor{}
	authz := authorization.NewServiceWithAuditor(nil, auditor)
	return NewDownloads(service, ratelimit.New(limit, time.Minute), authz), auditor
}

func downloadRequest(body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/public/events/token/downloads", strings.NewReader(body))
	request.SetPathValue("publicToken", "token")
	return request
}

func TestPublicDownloadsSuccess(t *testing.T) {
	handler, auditor := newDownloadsHandler(t, download.Scope{OrganizationID: "org-1", EventID: "event-1", DownloadsEnabled: true}, nil, 0)
	recorder := httptest.NewRecorder()
	handler.Public(recorder, downloadRequest(`{"photoIds":["`+readyPhoto+`"]}`))

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", recorder.Code)
	}
	var response publicDownloadResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(response.Downloads) != 1 || response.Downloads[0].PhotoID != readyPhoto || response.Downloads[0].URL == "" {
		t.Fatalf("unexpected downloads: %#v", response.Downloads)
	}
	if len(auditor.records) != 1 || auditor.records[0].Outcome != authorization.AuditSuccess {
		t.Fatalf("expected one success audit record, got %#v", auditor.records)
	}
}

func TestPublicDownloadsDisabledIsUniform404(t *testing.T) {
	handler, auditor := newDownloadsHandler(t, download.Scope{OrganizationID: "org-1", EventID: "event-1", DownloadsEnabled: false}, nil, 0)
	recorder := httptest.NewRecorder()
	handler.Public(recorder, downloadRequest(`{"photoIds":["`+readyPhoto+`"]}`))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
	if len(auditor.records) != 1 || auditor.records[0].Outcome != authorization.AuditDenied {
		t.Fatalf("expected one denied audit record, got %#v", auditor.records)
	}
}

func TestPublicDownloadsUnknownEventIs404(t *testing.T) {
	handler, auditor := newDownloadsHandler(t, download.Scope{}, errors.New("not found"), 0)
	recorder := httptest.NewRecorder()
	handler.Public(recorder, downloadRequest(`{"photoIds":["`+readyPhoto+`"]}`))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", recorder.Code)
	}
	if len(auditor.records) != 0 {
		t.Fatalf("unknown Event must not emit a tenant audit record, got %#v", auditor.records)
	}
}

func TestPublicDownloadsInvalidBodyIs400(t *testing.T) {
	handler, _ := newDownloadsHandler(t, download.Scope{OrganizationID: "org-1", EventID: "event-1", DownloadsEnabled: true}, nil, 0)
	recorder := httptest.NewRecorder()
	handler.Public(recorder, downloadRequest(`{"photoIds":[]}`))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty photoIds, got %d", recorder.Code)
	}
}

func TestPublicDownloadsRateLimited(t *testing.T) {
	handler, auditor := newDownloadsHandler(t, download.Scope{OrganizationID: "org-1", EventID: "event-1", DownloadsEnabled: true}, nil, 1)

	first := httptest.NewRecorder()
	handler.Public(first, downloadRequest(`{"photoIds":["`+readyPhoto+`"]}`))
	if first.Code != http.StatusOK {
		t.Fatalf("first request should succeed, got %d", first.Code)
	}

	second := httptest.NewRecorder()
	handler.Public(second, downloadRequest(`{"photoIds":["`+readyPhoto+`"]}`))
	if second.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be rate limited, got %d", second.Code)
	}
	rateLimited := false
	for _, record := range auditor.records {
		if record.Action == "download.rate_limited" && record.Outcome == authorization.AuditDenied {
			rateLimited = true
		}
	}
	if !rateLimited {
		t.Fatalf("rate-limited request must be audited, got %#v", auditor.records)
	}
}
