package photo

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	created CreateParams
	photo   Photo
	err     error
}

func (f *fakeRepository) Create(_ context.Context, params CreateParams) (Photo, error) {
	f.created = params
	return f.photo, f.err
}
func (f *fakeRepository) List(context.Context, string, string) ([]Photo, error) {
	return []Photo{f.photo}, f.err
}
func (f *fakeRepository) Find(context.Context, string, string, string) (Photo, error) {
	return f.photo, f.err
}
func (f *fakeRepository) Delete(context.Context, string, string, string) error { return f.err }
func (f *fakeRepository) Reprocess(context.Context, string, string, string) (Photo, error) {
	return f.photo, f.err
}

func TestServiceDerivesTrustedPhotoScope(t *testing.T) {
	repository := &fakeRepository{photo: Photo{ID: "photo-1", EventID: "event-1", Status: StatusPending}}
	service := NewService(repository)
	command, err := NewCreateCommand("photo.jpg", "image/jpeg", 12, "")
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), "org-1", "event-1", "user-1", command)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "photo-1" || repository.created.OrganizationID != "org-1" || repository.created.EventID != "event-1" || repository.created.CreatedByUserID != "user-1" {
		t.Fatalf("created = %#v, trusted params = %#v", created, repository.created)
	}
}

func TestServiceRejectsMissingTrustedPhotoScope(t *testing.T) {
	service := NewService(&fakeRepository{})
	command, err := NewCreateCommand("photo.jpg", "image/jpeg", 12, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), "", "event-1", "user-1", command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestObjectKeyUsesOnlyTrustedOpaqueScope(t *testing.T) {
	got, err := ObjectKey("org-1", "event-1", "photo-1")
	if err != nil {
		t.Fatal(err)
	}
	if got != "organizations/org-1/events/event-1/photos/photo-1/original" {
		t.Fatalf("object key = %q", got)
	}
	if _, err := ObjectKey("", "event-1", "photo-1"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}

func TestLifecycleTransitionsFailClosed(t *testing.T) {
	tests := []struct {
		name string
		from Status
		to   Status
		want bool
	}{
		{name: "pending to uploading", from: StatusPending, to: StatusUploading, want: true},
		{name: "uploading to uploaded", from: StatusUploading, to: StatusUploaded, want: true},
		{name: "uploaded to queued", from: StatusUploaded, to: StatusQueued, want: true},
		{name: "queued to processing", from: StatusQueued, to: StatusProcessing, want: true},
		{name: "processing to ready", from: StatusProcessing, to: StatusReady, want: true},
		{name: "processing to failed", from: StatusProcessing, to: StatusFailed, want: true},
		{name: "failed to queued for reprocess", from: StatusFailed, to: StatusQueued, want: true},
		{name: "active to deleted", from: StatusUploading, to: StatusDeleted, want: true},
		{name: "deleted remains deleted", from: StatusDeleted, to: StatusDeleted, want: true},
		{name: "ready cannot return to uploading", from: StatusReady, to: StatusUploading},
		{name: "unknown state", from: Status("unknown"), to: StatusFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := CanTransition(test.from, test.to); got != test.want {
				t.Fatalf("CanTransition(%q, %q) = %v, want %v", test.from, test.to, got, test.want)
			}
		})
	}
}

func TestNewCreateCommandNormalizesInformationalMetadata(t *testing.T) {
	command, err := NewCreateCommand("  wedding.jpg  ", "image/jpeg", 1024, "")
	if err != nil {
		t.Fatal(err)
	}
	if command.OriginalFilename != "wedding.jpg" || command.ContentType != "image/jpeg" {
		t.Fatalf("command = %#v", command)
	}
}

func TestNewCreateCommandRejectsInvalidMetadata(t *testing.T) {
	tests := []struct {
		name        string
		filename    string
		contentType string
		byteSize    int64
		checksum    string
	}{
		{name: "missing filename", contentType: "image/jpeg", byteSize: 1},
		{name: "unsupported type", filename: "photo.gif", contentType: "image/gif", byteSize: 1},
		{name: "empty object", filename: "photo.jpg", contentType: "image/jpeg"},
		{name: "invalid checksum", filename: "photo.jpg", contentType: "image/jpeg", byteSize: 1, checksum: "bad"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCreateCommand(test.filename, test.contentType, test.byteSize, test.checksum)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestPublicPhotoOmitsObjectKey(t *testing.T) {
	photo := Photo{ID: "photo-1", OrganizationID: "org-1", EventID: "event-1", ObjectKey: "secret", Status: StatusPending, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	view := photo.View()
	if view.ID != photo.ID || view.Status != photo.Status {
		t.Fatalf("view = %#v", view)
	}
}
