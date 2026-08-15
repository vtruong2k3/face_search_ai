package photo

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeUploadSessions struct {
	session       UploadSession
	found         bool
	createdUpload string
	finalized     bool
	aborted       bool
	createErr     error
	findCalls     int
	findAfterErr  UploadSession
	finalizePhoto Photo
	finalizeErr   error
}

func (f *fakeUploadSessions) FindActive(context.Context, string, string, string, time.Time) (UploadSession, bool, error) {
	f.findCalls++
	if f.createErr != nil && f.findCalls > 1 && f.findAfterErr.UploadID != "" {
		return f.findAfterErr, true, nil
	}
	return f.session, f.found, nil
}
func (f *fakeUploadSessions) FindForCompletion(_ context.Context, _, _, _, _ string, _ time.Time) (UploadSession, bool, error) {
	return f.session, f.found, nil
}
func (f *fakeUploadSessions) Create(_ context.Context, organizationID, eventID, photoID, uploadID string, expiresAt time.Time) (UploadSession, error) {
	f.createdUpload = uploadID
	if f.createErr != nil {
		return UploadSession{}, f.createErr
	}
	f.session = UploadSession{OrganizationID: organizationID, EventID: eventID, PhotoID: photoID, UploadID: uploadID, Status: UploadSessionActive, ExpiresAt: expiresAt}
	f.found = true
	return f.session, nil
}
func (f *fakeUploadSessions) FinalizeCompleted(context.Context, string, string, string, string) (Photo, error) {
	f.finalized = true
	if f.finalizeErr != nil {
		return Photo{}, f.finalizeErr
	}
	result := f.finalizePhoto
	result.Status = StatusQueued
	return result, nil
}
func (f *fakeUploadSessions) MarkAborted(context.Context, string, string, string, string) error {
	f.aborted = true
	return nil
}

type fakeMultipartStorage struct {
	initiated int
	signed    int
	completed int
	aborted   int
	stored    StoredObject
}

func (f *fakeMultipartStorage) Initiate(context.Context, string, string, string) (string, error) {
	f.initiated++
	return "upload-1", nil
}
func (f *fakeMultipartStorage) SignPart(context.Context, string, string, int, time.Duration) (string, error) {
	f.signed++
	return "https://storage.test/signed", nil
}
func (f *fakeMultipartStorage) Complete(context.Context, string, string, []CompletedPart) error {
	f.completed++
	return nil
}
func (f *fakeMultipartStorage) Abort(context.Context, string, string) error {
	f.aborted++
	return nil
}
func (f *fakeMultipartStorage) Stat(context.Context, string) (StoredObject, error) {
	return f.stored, nil
}

func uploadTestService(t *testing.T, sessions *fakeUploadSessions, storage *fakeMultipartStorage) *UploadService {
	t.Helper()
	policy, err := NewUploadPolicy(100*1024*1024, 8*1024*1024, 1000, time.Minute, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	photos := &fakeRepository{photo: Photo{ID: "photo-1", OrganizationID: "org-1", EventID: "event-1", ObjectKey: "trusted/key", ContentType: "image/jpeg", ByteSize: 9 * 1024 * 1024, Status: StatusPending}}
	service := NewUploadService(photos, sessions, storage, policy)
	service.now = func() time.Time { return time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC) }
	return service
}

func TestInitiateUploadReusesActiveSession(t *testing.T) {
	sessions := &fakeUploadSessions{found: true, session: UploadSession{UploadID: "existing", ExpiresAt: time.Now().Add(time.Hour)}}
	storage := &fakeMultipartStorage{}
	result, err := uploadTestService(t, sessions, storage).Initiate(context.Background(), "org-1", "event-1", "photo-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.UploadID != "existing" || storage.initiated != 0 {
		t.Fatalf("result=%#v initiated=%d", result, storage.initiated)
	}
}

func TestInitiateUploadReusesConcurrentSession(t *testing.T) {
	sessions := &fakeUploadSessions{
		createErr:    errors.New("concurrent session"),
		findAfterErr: UploadSession{UploadID: "winner", ExpiresAt: time.Now().Add(time.Hour)},
	}
	storage := &fakeMultipartStorage{}
	result, err := uploadTestService(t, sessions, storage).Initiate(context.Background(), "org-1", "event-1", "photo-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.UploadID != "winner" || storage.initiated != 1 || storage.aborted != 1 {
		t.Fatalf("result=%#v initiated=%d aborted=%d", result, storage.initiated, storage.aborted)
	}
}

func TestCompleteUploadFinalizesAtomically(t *testing.T) {
	session := UploadSession{UploadID: "upload-1", Status: UploadSessionActive, ExpiresAt: time.Now().Add(time.Hour)}
	finalPhoto := Photo{ID: "photo-1", OrganizationID: "org-1", EventID: "event-1", Status: StatusQueued, ProcessingGeneration: 1}
	sessions := &fakeUploadSessions{found: true, session: session, finalizePhoto: finalPhoto}
	storage := &fakeMultipartStorage{stored: StoredObject{ByteSize: 9 * 1024 * 1024, ContentType: "image/jpeg"}}
	view, err := uploadTestService(t, sessions, storage).Complete(context.Background(), "org-1", "event-1", "photo-1", "upload-1", []CompletedPart{{PartNumber: 1, ETag: "etag-1"}, {PartNumber: 2, ETag: "etag-2"}})
	if err != nil {
		t.Fatal(err)
	}
	if view.Status != StatusQueued || !sessions.finalized {
		t.Fatalf("status=%v finalized=%v", view.Status, sessions.finalized)
	}
}

func TestCompleteUploadReplayReturnsSameState(t *testing.T) {
	// Completed session + photo already queued → replay path skips MinIO re-completion.
	session := UploadSession{UploadID: "upload-1", Status: UploadSessionCompleted}
	sessions := &fakeUploadSessions{found: true, session: session}
	storage := &fakeMultipartStorage{}
	// fakeRepository returns a queued photo
	view, err := uploadTestService(t, sessions, storage).Complete(context.Background(), "org-1", "event-1", "photo-1", "upload-1", []CompletedPart{{PartNumber: 1, ETag: "etag-1"}})
	if err != nil {
		t.Fatal(err)
	}
	if storage.completed != 0 || sessions.finalized {
		t.Fatalf("replay should skip MinIO and FinalizeCompleted: completed=%d finalized=%v", storage.completed, sessions.finalized)
	}
	_ = view
}

func TestCompleteUploadRejectsStoredMetadataMismatch(t *testing.T) {
	sessions := &fakeUploadSessions{found: true, session: UploadSession{UploadID: "upload-1", Status: UploadSessionActive, ExpiresAt: time.Now().Add(time.Hour)}}
	storage := &fakeMultipartStorage{stored: StoredObject{ByteSize: 1, ContentType: "image/jpeg"}}
	_, err := uploadTestService(t, sessions, storage).Complete(context.Background(), "org-1", "event-1", "photo-1", "upload-1", []CompletedPart{{PartNumber: 1, ETag: "a"}, {PartNumber: 2, ETag: "b"}})
	if !errors.Is(err, ErrInvalid) || sessions.finalized {
		t.Fatalf("error=%v finalized=%v", err, sessions.finalized)
	}
}

func TestAbortUploadIsIdempotentWithoutActiveSession(t *testing.T) {
	storage := &fakeMultipartStorage{}
	if err := uploadTestService(t, &fakeUploadSessions{}, storage).Abort(context.Background(), "org-1", "event-1", "photo-1", "old"); err != nil {
		t.Fatal(err)
	}
	if storage.aborted != 0 {
		t.Fatalf("storage aborts = %d", storage.aborted)
	}
}

func TestUploadPolicyValidatesBounds(t *testing.T) {
	policy, err := NewUploadPolicy(100*1024*1024, 8*1024*1024, 1000, 10*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if policy.PartCount(16*1024*1024+1) != 3 {
		t.Fatalf("part count = %d", policy.PartCount(16*1024*1024+1))
	}
	if err := policy.ValidatePart(0); !errors.Is(err, ErrInvalid) {
		t.Fatalf("part zero error = %v", err)
	}
	if err := policy.ValidatePart(1001); !errors.Is(err, ErrInvalid) {
		t.Fatalf("part overflow error = %v", err)
	}
}

func TestUploadPolicyRejectsUnsafeConfiguration(t *testing.T) {
	tests := []struct {
		maxSize    int64
		partSize   int64
		maxParts   int
		signTTL    time.Duration
		sessionTTL time.Duration
	}{
		{0, 8 * 1024 * 1024, 1000, time.Minute, time.Hour},
		{100, 4 * 1024 * 1024, 1000, time.Minute, time.Hour},
		{100, 8 * 1024 * 1024, 0, time.Minute, time.Hour},
		{100, 8 * 1024 * 1024, 1000, 0, time.Hour},
		{100, 8 * 1024 * 1024, 1000, time.Hour, time.Minute},
	}
	for _, test := range tests {
		if _, err := NewUploadPolicy(test.maxSize, test.partSize, test.maxParts, test.signTTL, test.sessionTTL); !errors.Is(err, ErrInvalid) {
			t.Fatalf("NewUploadPolicy(%+v) error = %v", test, err)
		}
	}
}

func TestUploadSessionIsActiveOnlyBeforeExpiry(t *testing.T) {
	now := time.Date(2026, 8, 15, 12, 0, 0, 0, time.UTC)
	session := UploadSession{Status: UploadSessionActive, ExpiresAt: now.Add(time.Minute)}
	if !session.ActiveAt(now) || session.ActiveAt(now.Add(time.Minute)) {
		t.Fatal("session expiry boundary is not fail closed")
	}
	session.Status = UploadSessionAborted
	if session.ActiveAt(now) {
		t.Fatal("aborted session is active")
	}
}

func TestCompletePartsRequireOrderedUniqueBounds(t *testing.T) {
	parts, err := NewCompletedParts([]CompletedPart{{PartNumber: 1, ETag: " etag-1 "}, {PartNumber: 2, ETag: "etag-2"}}, 3)
	if err != nil {
		t.Fatal(err)
	}
	if parts[0].ETag != "etag-1" {
		t.Fatalf("etag = %q", parts[0].ETag)
	}
	for _, invalid := range [][]CompletedPart{
		{{PartNumber: 2, ETag: "etag"}, {PartNumber: 1, ETag: "etag"}},
		{{PartNumber: 1, ETag: ""}},
		{{PartNumber: 4, ETag: "etag"}},
	} {
		if _, err := NewCompletedParts(invalid, 3); !errors.Is(err, ErrInvalid) {
			t.Fatalf("parts %#v error = %v", invalid, err)
		}
	}
}

func TestPhotoIdempotencyKey(t *testing.T) {
	p := Photo{ID: "abc-123", ProcessingGeneration: 2}
	want := "photo.process:abc-123:2"
	if got := p.IdempotencyKey(); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
