package download

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeScopeResolver struct {
	scope Scope
	err   error
}

func (f fakeScopeResolver) FindPublicDownloadScope(context.Context, string, time.Time) (Scope, error) {
	return f.scope, f.err
}

type resolverCall struct{ organizationID, eventID, photoID string }

type fakePhotoResolver struct {
	objects map[string]DownloadableObject
	calls   []resolverCall
}

func (f *fakePhotoResolver) FindDownloadable(_ context.Context, organizationID, eventID, photoID string) (DownloadableObject, error) {
	f.calls = append(f.calls, resolverCall{organizationID, eventID, photoID})
	object, ok := f.objects[photoID]
	if !ok {
		return DownloadableObject{}, errors.New("not found")
	}
	return object, nil
}

type signerCall struct {
	objectKey   string
	disposition string
	ttl         time.Duration
}

type fakeSigner struct {
	calls     []signerCall
	expiresAt time.Time
	err       error
}

func (f *fakeSigner) PresignGet(_ context.Context, objectKey, disposition string, ttl time.Duration) (string, time.Time, error) {
	f.calls = append(f.calls, signerCall{objectKey, disposition, ttl})
	if f.err != nil {
		return "", time.Time{}, f.err
	}
	return "signed:" + objectKey, f.expiresAt, nil
}

type fakeRecorder struct{ entries []AuditEntry }

func (f *fakeRecorder) Record(_ context.Context, entry AuditEntry) error {
	f.entries = append(f.entries, entry)
	return nil
}

const (
	photoA = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	photoB = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	photoC = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

func newService(t *testing.T, scopes ScopeResolver, photos PhotoResolver, signer ObjectURLSigner, recorder Recorder, ttl time.Duration, maxBulk int) *Service {
	t.Helper()
	service, err := NewService(scopes, photos, signer, recorder, ttl, maxBulk)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return service
}

func enabledScope() fakeScopeResolver {
	return fakeScopeResolver{scope: Scope{OrganizationID: "org-1", EventID: "event-1", DownloadsEnabled: true}}
}

func TestIssueAllowedSignsScopedShortLivedURL(t *testing.T) {
	expiry := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	photos := &fakePhotoResolver{objects: map[string]DownloadableObject{photoA: {ObjectKey: "organizations/org-1/events/event-1/photos/a/original", ContentType: "image/jpeg"}}}
	signer := &fakeSigner{expiresAt: expiry}
	recorder := &fakeRecorder{}
	service := newService(t, enabledScope(), photos, signer, recorder, 2*time.Minute, 50)

	result, err := service.Issue(context.Background(), "token", []string{photoA})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if result.Decision != DecisionAllowed || result.Kind != KindSingle || len(result.Grants) != 1 {
		t.Fatalf("unexpected result: %#v", result)
	}
	grant := result.Grants[0]
	if grant.PhotoID != photoA || grant.URL != "signed:organizations/org-1/events/event-1/photos/a/original" {
		t.Fatalf("unexpected grant: %#v", grant)
	}
	if !grant.ExpiresAt.Equal(expiry) {
		t.Fatalf("grant expiry not propagated: %v", grant.ExpiresAt)
	}
	if len(signer.calls) != 1 || signer.calls[0].ttl != 2*time.Minute {
		t.Fatalf("signer not called with configured short TTL: %#v", signer.calls)
	}
	if signer.calls[0].disposition != `attachment; filename="`+photoA+`.jpg"` {
		t.Fatalf("unexpected disposition: %q", signer.calls[0].disposition)
	}
}

func TestIssueUsesTrustedScopeForPhotoResolution(t *testing.T) {
	photos := &fakePhotoResolver{objects: map[string]DownloadableObject{photoA: {ObjectKey: "key", ContentType: "image/png"}}}
	service := newService(t, enabledScope(), photos, &fakeSigner{}, &fakeRecorder{}, time.Minute, 50)

	if _, err := service.Issue(context.Background(), "token", []string{photoA}); err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if len(photos.calls) != 1 || photos.calls[0].organizationID != "org-1" || photos.calls[0].eventID != "event-1" {
		t.Fatalf("photo resolution did not use trusted scope: %#v", photos.calls)
	}
}

func TestIssueDeniedWhenDownloadsDisabled(t *testing.T) {
	signer := &fakeSigner{}
	recorder := &fakeRecorder{}
	scopes := fakeScopeResolver{scope: Scope{OrganizationID: "org-1", EventID: "event-1", DownloadsEnabled: false}}
	service := newService(t, scopes, &fakePhotoResolver{}, signer, recorder, time.Minute, 50)

	result, err := service.Issue(context.Background(), "token", []string{photoA})
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("expected ErrNotAvailable, got %v", err)
	}
	if result.Decision != DecisionDenied || result.DenialCode != DenialDownloadsDisabled {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(signer.calls) != 0 {
		t.Fatalf("no URL should be signed when downloads are disabled")
	}
	if len(recorder.entries) != 1 || recorder.entries[0].Decision != DecisionDenied || recorder.entries[0].DenialCode != DenialDownloadsDisabled {
		t.Fatalf("expected one denied audit entry, got %#v", recorder.entries)
	}
	// A single-kind denial must carry the requested photo id so the audit row
	// satisfies the download_records single-photo constraint.
	if recorder.entries[0].Kind != KindSingle || recorder.entries[0].PhotoID != photoA {
		t.Fatalf("single denial must record its photo id, got %#v", recorder.entries[0])
	}
}

// A photo from another Event or tenant is not returned by the scoped resolver.
// The whole request must be rejected uniformly with a recorded scope violation.
func TestIssueDeniedOnCrossEventScopeViolation(t *testing.T) {
	signer := &fakeSigner{}
	recorder := &fakeRecorder{}
	photos := &fakePhotoResolver{objects: map[string]DownloadableObject{photoA: {ObjectKey: "key", ContentType: "image/jpeg"}}}
	service := newService(t, enabledScope(), photos, signer, recorder, time.Minute, 50)

	result, err := service.Issue(context.Background(), "token", []string{photoA, photoB})
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("expected ErrNotAvailable, got %v", err)
	}
	if result.Decision != DecisionDenied || result.DenialCode != DenialScopeViolation {
		t.Fatalf("unexpected result: %#v", result)
	}
	if len(result.Grants) != 0 {
		t.Fatalf("no grants should be returned on scope violation")
	}
	if len(recorder.entries) == 0 || recorder.entries[len(recorder.entries)-1].DenialCode != DenialScopeViolation {
		t.Fatalf("expected a scope violation audit entry, got %#v", recorder.entries)
	}
}

func TestIssueUnknownEventIsUniformAndUnaudited(t *testing.T) {
	recorder := &fakeRecorder{}
	scopes := fakeScopeResolver{err: errors.New("not found")}
	service := newService(t, scopes, &fakePhotoResolver{}, &fakeSigner{}, recorder, time.Minute, 50)

	result, err := service.Issue(context.Background(), "token", []string{photoA})
	if !errors.Is(err, ErrNotAvailable) {
		t.Fatalf("expected ErrNotAvailable, got %v", err)
	}
	if result.OrganizationID != "" {
		t.Fatalf("unknown Event must not resolve a tenant scope: %#v", result)
	}
	if len(recorder.entries) != 0 {
		t.Fatalf("unknown Event must not write a tenant-scoped audit entry: %#v", recorder.entries)
	}
}

func TestIssueBulkAllowedAndAudited(t *testing.T) {
	photos := &fakePhotoResolver{objects: map[string]DownloadableObject{
		photoA: {ObjectKey: "key-a", ContentType: "image/jpeg"},
		photoB: {ObjectKey: "key-b", ContentType: "image/png"},
	}}
	recorder := &fakeRecorder{}
	service := newService(t, enabledScope(), photos, &fakeSigner{expiresAt: time.Now()}, recorder, time.Minute, 50)

	result, err := service.Issue(context.Background(), "token", []string{photoA, photoB})
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if result.Kind != KindBulk || len(result.Grants) != 2 {
		t.Fatalf("unexpected bulk result: %#v", result)
	}
	allowed := 0
	for _, entry := range recorder.entries {
		if entry.Decision == DecisionAllowed {
			allowed++
		}
	}
	if allowed != 2 {
		t.Fatalf("expected two allowed audit entries, got %#v", recorder.entries)
	}
}

func TestIssueEnforcesBulkBounds(t *testing.T) {
	service := newService(t, enabledScope(), &fakePhotoResolver{}, &fakeSigner{}, &fakeRecorder{}, time.Minute, 2)

	if _, err := service.Issue(context.Background(), "token", nil); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("empty request should be invalid, got %v", err)
	}
	if _, err := service.Issue(context.Background(), "token", []string{photoA, photoB, photoC}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("over-cap request should be invalid, got %v", err)
	}
}

func TestIssueTransientSignerFailureIsUnavailable(t *testing.T) {
	photos := &fakePhotoResolver{objects: map[string]DownloadableObject{photoA: {ObjectKey: "key", ContentType: "image/jpeg"}}}
	signer := &fakeSigner{err: errors.New("minio down")}
	service := newService(t, enabledScope(), photos, signer, &fakeRecorder{}, time.Minute, 50)

	if _, err := service.Issue(context.Background(), "token", []string{photoA}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestNormalizePhotoIDsValidatesDeduplicatesAndBounds(t *testing.T) {
	if _, err := NormalizePhotoIDs([]string{"not-a-uuid"}, 10); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid uuid should be rejected, got %v", err)
	}
	ids, err := NormalizePhotoIDs([]string{photoA, photoA, photoB}, 10)
	if err != nil {
		t.Fatalf("normalize: %v", err)
	}
	if len(ids) != 2 {
		t.Fatalf("duplicates should be removed: %#v", ids)
	}
}

func TestNewServiceRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewService(nil, &fakePhotoResolver{}, &fakeSigner{}, &fakeRecorder{}, time.Minute, 50); err == nil {
		t.Fatal("nil scope resolver should fail")
	}
	if _, err := NewService(enabledScope(), &fakePhotoResolver{}, &fakeSigner{}, &fakeRecorder{}, 0, 50); err == nil {
		t.Fatal("non-positive TTL should fail")
	}
	if _, err := NewService(enabledScope(), &fakePhotoResolver{}, &fakeSigner{}, &fakeRecorder{}, time.Minute, MaxBulkHardCap+1); err == nil {
		t.Fatal("max bulk over the hard cap should fail")
	}
}
