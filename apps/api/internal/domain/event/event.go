package event

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"time"
)

var ErrInvalid = errors.New("invalid event")

type Visibility string

const (
	VisibilityPrivate Visibility = "private"
	VisibilityPublic  Visibility = "public"
)

type Status string

const (
	StatusActive   Status = "active"
	StatusArchived Status = "archived"
)

type Event struct {
	ID               string     `json:"id"`
	OrganizationID   string     `json:"organizationId"`
	Name             string     `json:"name"`
	Visibility       Visibility `json:"visibility"`
	Status           Status     `json:"status"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	DownloadsEnabled bool       `json:"downloadsEnabled"`
	MatchThreshold   *float64   `json:"matchThreshold"`
	PublicToken      string     `json:"-"`
	CreatedByUserID  string     `json:"createdByUserId"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

type PublicEvent struct {
	Name             string     `json:"name"`
	ExpiresAt        *time.Time `json:"expiresAt"`
	DownloadsEnabled bool       `json:"downloadsEnabled"`
}

func (e Event) IsPubliclyEligible(now time.Time) bool {
	return e.Visibility == VisibilityPublic && e.Status == StatusActive && e.PublicToken != "" && (e.ExpiresAt == nil || e.ExpiresAt.After(now))
}

func CanonicalPublicURL(origin, token string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || token == "" {
		return "", ErrInvalid
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/e/" + url.PathEscape(token)
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

type CreateCommand struct {
	Name             string
	Visibility       Visibility
	ExpiresAt        *time.Time
	DownloadsEnabled bool
	MatchThreshold   *float64
}

type CreateParams struct {
	OrganizationID  string
	CreatedByUserID string
	CreateCommand
}

type UpdateCommand struct {
	Name             *string
	Visibility       *Visibility
	ExpiresAt        **time.Time
	DownloadsEnabled *bool
	MatchThreshold   **float64
}

type ProcessingStatus struct {
	EventID     string `json:"eventId"`
	ActiveTotal int64  `json:"activeTotal"`
	Pending     int64  `json:"pending"`
	Uploading   int64  `json:"uploading"`
	Uploaded    int64  `json:"uploaded"`
	Queued      int64  `json:"queued"`
	Processing  int64  `json:"processing"`
	Ready       int64  `json:"ready"`
	Failed      int64  `json:"failed"`
	Deleted     int64  `json:"deleted"`
}

type Repository interface {
	Create(context.Context, CreateParams) (Event, error)
	List(context.Context, string) ([]Event, error)
	Find(context.Context, string, string) (Event, error)
	Update(context.Context, string, string, UpdateCommand) (Event, error)
	Archive(context.Context, string, string) error
	Status(context.Context, string, string) (ProcessingStatus, error)
	FindPublic(context.Context, string, time.Time) (PublicEvent, error)
}

type Service struct {
	repository Repository
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func NewCreateCommand(name string, visibility Visibility, expiresAt *time.Time, downloadsEnabled bool, matchThreshold *float64) (CreateCommand, error) {
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 200 {
		return CreateCommand{}, ErrInvalid
	}
	if visibility != VisibilityPrivate && visibility != VisibilityPublic {
		return CreateCommand{}, ErrInvalid
	}
	if expiresAt != nil && !expiresAt.After(time.Now()) {
		return CreateCommand{}, ErrInvalid
	}
	if matchThreshold != nil && (*matchThreshold < -1 || *matchThreshold > 1) {
		return CreateCommand{}, ErrInvalid
	}
	return CreateCommand{
		Name: name, Visibility: visibility, ExpiresAt: expiresAt,
		DownloadsEnabled: downloadsEnabled, MatchThreshold: matchThreshold,
	}, nil
}

func NewUpdateCommand(name *string, visibility *Visibility, expiresAt **time.Time, downloadsEnabled *bool, matchThreshold **float64) (UpdateCommand, error) {
	if name != nil {
		normalized := strings.TrimSpace(*name)
		if normalized == "" || len(normalized) > 200 {
			return UpdateCommand{}, ErrInvalid
		}
		name = &normalized
	}
	if visibility != nil && *visibility != VisibilityPrivate && *visibility != VisibilityPublic {
		return UpdateCommand{}, ErrInvalid
	}
	if expiresAt != nil && *expiresAt != nil && !(*expiresAt).After(time.Now()) {
		return UpdateCommand{}, ErrInvalid
	}
	if matchThreshold != nil && *matchThreshold != nil && (**matchThreshold < -1 || **matchThreshold > 1) {
		return UpdateCommand{}, ErrInvalid
	}
	return UpdateCommand{
		Name: name, Visibility: visibility, ExpiresAt: expiresAt,
		DownloadsEnabled: downloadsEnabled, MatchThreshold: matchThreshold,
	}, nil
}

func (s *Service) Create(ctx context.Context, organizationID, actorUserID string, command CreateCommand) (Event, error) {
	if organizationID == "" || actorUserID == "" {
		return Event{}, ErrInvalid
	}
	return s.repository.Create(ctx, CreateParams{
		OrganizationID: organizationID, CreatedByUserID: actorUserID, CreateCommand: command,
	})
}

func (s *Service) List(ctx context.Context, organizationID string) ([]Event, error) {
	if organizationID == "" {
		return nil, ErrInvalid
	}
	return s.repository.List(ctx, organizationID)
}

func (s *Service) Find(ctx context.Context, organizationID, eventID string) (Event, error) {
	if organizationID == "" || eventID == "" {
		return Event{}, ErrInvalid
	}
	return s.repository.Find(ctx, organizationID, eventID)
}

func (s *Service) Update(ctx context.Context, organizationID, eventID string, command UpdateCommand) (Event, error) {
	if organizationID == "" || eventID == "" {
		return Event{}, ErrInvalid
	}
	return s.repository.Update(ctx, organizationID, eventID, command)
}

func (s *Service) Archive(ctx context.Context, organizationID, eventID string) error {
	if organizationID == "" || eventID == "" {
		return ErrInvalid
	}
	return s.repository.Archive(ctx, organizationID, eventID)
}

func (s *Service) FindPublic(ctx context.Context, token string, now time.Time) (PublicEvent, error) {
	if token == "" || len(token) > 200 {
		return PublicEvent{}, ErrInvalid
	}
	return s.repository.FindPublic(ctx, token, now)
}

func (s *Service) Status(ctx context.Context, organizationID, eventID string) (ProcessingStatus, error) {
	if organizationID == "" || eventID == "" {
		return ProcessingStatus{}, ErrInvalid
	}
	return s.repository.Status(ctx, organizationID, eventID)
}
