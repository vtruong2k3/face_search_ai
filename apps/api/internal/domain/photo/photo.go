package photo

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid photo")

const MaxByteSize int64 = 100 * 1024 * 1024

type Status string

const (
	StatusPending    Status = "pending"
	StatusUploading  Status = "uploading"
	StatusUploaded   Status = "uploaded"
	StatusQueued     Status = "queued"
	StatusProcessing Status = "processing"
	StatusReady      Status = "ready"
	StatusFailed     Status = "failed"
	StatusDeleted    Status = "deleted"
)

type Photo struct {
	ID                   string
	OrganizationID       string
	EventID              string
	ObjectKey            string
	OriginalFilename     string
	ContentType          string
	ByteSize             int64
	ChecksumSHA256       string
	Status               Status
	FailureCode          string
	ProcessingGeneration int
	CreatedByUserID      string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// IdempotencyKey returns the deterministic outbox key for the current processing generation.
func (p Photo) IdempotencyKey() string {
	return "photo.process:" + p.ID + ":" + itoa(p.ProcessingGeneration)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	buf := make([]byte, 0, 10)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}

type View struct {
	ID               string    `json:"id"`
	EventID          string    `json:"eventId"`
	OriginalFilename string    `json:"originalFilename"`
	ContentType      string    `json:"contentType"`
	ByteSize         int64     `json:"byteSize"`
	Status           Status    `json:"status"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
}

func (p Photo) View() View {
	return View{ID: p.ID, EventID: p.EventID, OriginalFilename: p.OriginalFilename, ContentType: p.ContentType, ByteSize: p.ByteSize, Status: p.Status, CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

type CreateCommand struct {
	OriginalFilename string
	ContentType      string
	ByteSize         int64
	ChecksumSHA256   string
}

type CreateParams struct {
	OrganizationID  string
	EventID         string
	CreatedByUserID string
	CreateCommand
}

type Repository interface {
	Create(context.Context, CreateParams) (Photo, error)
	List(context.Context, string, string) ([]Photo, error)
	Find(context.Context, string, string, string) (Photo, error)
	Delete(context.Context, string, string, string) error
	// Reprocess transitions a failed photo to queued, increments processing_generation,
	// and inserts a versioned outbox message atomically. Idempotent: returns ErrConflict
	// if the photo is not in the failed state.
	Reprocess(context.Context, string, string, string) (Photo, error)
}

type Service struct{ repository Repository }

func NewService(repository Repository) *Service { return &Service{repository: repository} }

var checksumPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

func NewCreateCommand(filename, contentType string, byteSize int64, checksum string) (CreateCommand, error) {
	filename = strings.TrimSpace(filename)
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	checksum = strings.ToLower(strings.TrimSpace(checksum))
	if filename == "" || len(filename) > 255 || (contentType != "image/jpeg" && contentType != "image/png" && contentType != "image/webp") || byteSize <= 0 || byteSize > MaxByteSize {
		return CreateCommand{}, ErrInvalid
	}
	if checksum != "" && !checksumPattern.MatchString(checksum) {
		return CreateCommand{}, ErrInvalid
	}
	return CreateCommand{OriginalFilename: filename, ContentType: contentType, ByteSize: byteSize, ChecksumSHA256: checksum}, nil
}

func ObjectKey(organizationID, eventID, photoID string) (string, error) {
	if !safeSegment(organizationID) || !safeSegment(eventID) || !safeSegment(photoID) {
		return "", ErrInvalid
	}
	return fmt.Sprintf("organizations/%s/events/%s/photos/%s/original", organizationID, eventID, photoID), nil
}

func safeSegment(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && !strings.ContainsAny(value, `/\\`)
}

func CanTransition(from, to Status) bool {
	if from == StatusDeleted {
		return to == StatusDeleted
	}
	if to == StatusDeleted {
		return knownStatus(from)
	}
	allowed := map[Status]Status{
		StatusPending: StatusUploading, StatusUploading: StatusUploaded,
		StatusUploaded: StatusQueued, StatusQueued: StatusProcessing,
		StatusFailed: StatusQueued,
	}
	if from == StatusProcessing {
		return to == StatusReady || to == StatusFailed
	}
	return allowed[from] == to
}

func knownStatus(status Status) bool {
	switch status {
	case StatusPending, StatusUploading, StatusUploaded, StatusQueued, StatusProcessing, StatusReady, StatusFailed, StatusDeleted:
		return true
	default:
		return false
	}
}

func (s *Service) Create(ctx context.Context, organizationID, eventID, actorUserID string, command CreateCommand) (View, error) {
	if organizationID == "" || eventID == "" || actorUserID == "" {
		return View{}, ErrInvalid
	}
	created, err := s.repository.Create(ctx, CreateParams{OrganizationID: organizationID, EventID: eventID, CreatedByUserID: actorUserID, CreateCommand: command})
	return created.View(), err
}

func (s *Service) List(ctx context.Context, organizationID, eventID string) ([]View, error) {
	if organizationID == "" || eventID == "" {
		return nil, ErrInvalid
	}
	photos, err := s.repository.List(ctx, organizationID, eventID)
	if err != nil {
		return nil, err
	}
	views := make([]View, 0, len(photos))
	for _, item := range photos {
		views = append(views, item.View())
	}
	return views, nil
}

func (s *Service) Find(ctx context.Context, organizationID, eventID, photoID string) (View, error) {
	if organizationID == "" || eventID == "" || photoID == "" {
		return View{}, ErrInvalid
	}
	result, err := s.repository.Find(ctx, organizationID, eventID, photoID)
	return result.View(), err
}

func (s *Service) Delete(ctx context.Context, organizationID, eventID, photoID string) error {
	if organizationID == "" || eventID == "" || photoID == "" {
		return ErrInvalid
	}
	return s.repository.Delete(ctx, organizationID, eventID, photoID)
}

func (s *Service) Reprocess(ctx context.Context, organizationID, eventID, photoID string) (View, error) {
	if organizationID == "" || eventID == "" || photoID == "" {
		return View{}, ErrInvalid
	}
	result, err := s.repository.Reprocess(ctx, organizationID, eventID, photoID)
	return result.View(), err
}
