package event

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRepository struct {
	created CreateParams
	event   Event
	err     error
}

func (f *fakeRepository) Create(_ context.Context, params CreateParams) (Event, error) {
	f.created = params
	return f.event, f.err
}
func (f *fakeRepository) List(context.Context, string) ([]Event, error)       { return nil, f.err }
func (f *fakeRepository) Find(context.Context, string, string) (Event, error) { return f.event, f.err }
func (f *fakeRepository) Update(context.Context, string, string, UpdateCommand) (Event, error) {
	return f.event, f.err
}
func (f *fakeRepository) Archive(context.Context, string, string) error { return f.err }
func (f *fakeRepository) Status(context.Context, string, string) (ProcessingStatus, error) {
	return ProcessingStatus{}, f.err
}

func TestCreateValidatesAndNormalizesMutableFields(t *testing.T) {
	expiresAt := time.Now().UTC().Add(time.Hour)
	threshold := 0.45
	command, err := NewCreateCommand("  Summer portraits  ", VisibilityPublic, &expiresAt, true, &threshold)
	if err != nil {
		t.Fatal(err)
	}
	if command.Name != "Summer portraits" || command.Visibility != VisibilityPublic {
		t.Fatalf("command = %#v", command)
	}
}

func TestCreateRejectsInvalidInput(t *testing.T) {
	past := time.Now().UTC().Add(-time.Minute)
	tooHigh := 1.01
	tests := []struct {
		name       string
		eventName  string
		visibility Visibility
		expiresAt  *time.Time
		threshold  *float64
	}{
		{name: "empty name", eventName: "   ", visibility: VisibilityPrivate},
		{name: "unknown visibility", eventName: "Event", visibility: Visibility("unknown")},
		{name: "past expiry", eventName: "Event", visibility: VisibilityPrivate, expiresAt: &past},
		{name: "threshold above cosine range", eventName: "Event", visibility: VisibilityPrivate, threshold: &tooHigh},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewCreateCommand(test.eventName, test.visibility, test.expiresAt, false, test.threshold)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("error = %v, want ErrInvalid", err)
			}
		})
	}
}

func TestServiceDerivesTrustedOwnership(t *testing.T) {
	repository := &fakeRepository{event: Event{ID: "event-1"}}
	service := NewService(repository)
	command, err := NewCreateCommand("Event", VisibilityPrivate, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	created, err := service.Create(context.Background(), "org-1", "user-1", command)
	if err != nil {
		t.Fatal(err)
	}
	if created.ID != "event-1" {
		t.Fatalf("event = %#v", created)
	}
	if repository.created.OrganizationID != "org-1" || repository.created.CreatedByUserID != "user-1" {
		t.Fatalf("trusted create params = %#v", repository.created)
	}
}

func TestServiceRejectsMissingTrustedOwnership(t *testing.T) {
	service := NewService(&fakeRepository{})
	command, err := NewCreateCommand("Event", VisibilityPrivate, nil, false, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.Create(context.Background(), "", "user-1", command); !errors.Is(err, ErrInvalid) {
		t.Fatalf("error = %v, want ErrInvalid", err)
	}
}
