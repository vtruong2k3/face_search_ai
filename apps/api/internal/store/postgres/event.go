package postgres

import (
	"context"
	"time"

	"github.com/face-search-ai/api/internal/domain/event"
	"github.com/face-search-ai/api/internal/store"
)

type EventRepository struct {
	db *Store
}

func NewEventRepository(db *Store) *EventRepository {
	return &EventRepository{db: db}
}

func (r *EventRepository) Create(ctx context.Context, params event.CreateParams) (event.Event, error) {
	var result event.Event
	err := r.db.QueryRow(ctx, `
		INSERT INTO events (
			organization_id, name, visibility, expires_at,
			downloads_enabled, match_threshold, created_by_user_id, public_token
		) VALUES ($1, $2, $3, $4, $5, $6, $7,
			CASE WHEN $3 = 'public' THEN encode(gen_random_bytes(32), 'hex') ELSE NULL END)
		RETURNING id, organization_id, name, visibility, status, expires_at,
			downloads_enabled, match_threshold, created_by_user_id, created_at, updated_at`,
		params.OrganizationID, params.Name, params.Visibility, params.ExpiresAt,
		params.DownloadsEnabled, params.MatchThreshold, params.CreatedByUserID,
	).Scan(&result.ID, &result.OrganizationID, &result.Name, &result.Visibility,
		&result.Status, &result.ExpiresAt, &result.DownloadsEnabled, &result.MatchThreshold,
		&result.CreatedByUserID, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return event.Event{}, MapError(err)
	}
	return result, nil
}

func (r *EventRepository) List(ctx context.Context, organizationID string) ([]event.Event, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, organization_id, name, visibility, status, expires_at,
			downloads_enabled, match_threshold, created_by_user_id, created_at, updated_at
		FROM events
		WHERE organization_id = $1 AND status = 'active'
		ORDER BY created_at DESC, id`, organizationID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	results := make([]event.Event, 0)
	for rows.Next() {
		var result event.Event
		if err := rows.Scan(&result.ID, &result.OrganizationID, &result.Name, &result.Visibility,
			&result.Status, &result.ExpiresAt, &result.DownloadsEnabled, &result.MatchThreshold,
			&result.CreatedByUserID, &result.CreatedAt, &result.UpdatedAt); err != nil {
			return nil, MapError(err)
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	return results, nil
}

func (r *EventRepository) Find(ctx context.Context, organizationID, eventID string) (event.Event, error) {
	var result event.Event
	err := r.db.QueryRow(ctx, `
		SELECT id, organization_id, name, visibility, status, expires_at,
			downloads_enabled, match_threshold, created_by_user_id, created_at, updated_at
		FROM events
		WHERE organization_id = $1 AND id = $2 AND status = 'active'`, organizationID, eventID,
	).Scan(&result.ID, &result.OrganizationID, &result.Name, &result.Visibility,
		&result.Status, &result.ExpiresAt, &result.DownloadsEnabled, &result.MatchThreshold,
		&result.CreatedByUserID, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return event.Event{}, MapError(err)
	}
	return result, nil
}

func (r *EventRepository) Update(ctx context.Context, organizationID, eventID string, command event.UpdateCommand) (event.Event, error) {
	current, err := r.Find(ctx, organizationID, eventID)
	if err != nil {
		return event.Event{}, err
	}
	if command.Name != nil {
		current.Name = *command.Name
	}
	if command.Visibility != nil {
		current.Visibility = *command.Visibility
	}
	if command.ExpiresAt != nil {
		current.ExpiresAt = *command.ExpiresAt
	}
	if command.DownloadsEnabled != nil {
		current.DownloadsEnabled = *command.DownloadsEnabled
	}
	if command.MatchThreshold != nil {
		current.MatchThreshold = *command.MatchThreshold
	}
	err = r.db.QueryRow(ctx, `
		UPDATE events SET name = $3, visibility = $4, expires_at = $5,
			downloads_enabled = $6, match_threshold = $7,
			public_token = CASE WHEN $4 = 'public' THEN coalesce(public_token, encode(gen_random_bytes(32), 'hex')) ELSE public_token END,
			updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'active'
		RETURNING id, organization_id, name, visibility, status, expires_at,
			downloads_enabled, match_threshold, created_by_user_id, created_at, updated_at`,
		organizationID, eventID, current.Name, current.Visibility, current.ExpiresAt,
		current.DownloadsEnabled, current.MatchThreshold,
	).Scan(&current.ID, &current.OrganizationID, &current.Name, &current.Visibility,
		&current.Status, &current.ExpiresAt, &current.DownloadsEnabled, &current.MatchThreshold,
		&current.CreatedByUserID, &current.CreatedAt, &current.UpdatedAt)
	if err != nil {
		return event.Event{}, MapError(err)
	}
	return current, nil
}

func (r *EventRepository) Archive(ctx context.Context, organizationID, eventID string) error {
	tag, err := r.db.Exec(ctx, `UPDATE events SET status = 'archived', updated_at = now()
		WHERE organization_id = $1 AND id = $2 AND status = 'active'`, organizationID, eventID)
	if err != nil {
		return MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *EventRepository) FindPublic(ctx context.Context, token string, now time.Time) (event.PublicEvent, error) {
	var result event.PublicEvent
	err := r.db.QueryRow(ctx, `
		SELECT name, expires_at, downloads_enabled
		FROM events
		WHERE public_token = $1
		  AND visibility = 'public'
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > $2)`, token, now,
	).Scan(&result.Name, &result.ExpiresAt, &result.DownloadsEnabled)
	if err != nil {
		return event.PublicEvent{}, MapError(err)
	}
	return result, nil
}

func (r *EventRepository) FindPublicSearchScope(ctx context.Context, token string, now time.Time) (event.PublicSearchScope, error) {
	var result event.PublicSearchScope
	err := r.db.QueryRow(ctx, `
		SELECT organization_id, id
		FROM events
		WHERE public_token = $1
		  AND visibility = 'public'
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > $2)`, token, now,
	).Scan(&result.OrganizationID, &result.EventID)
	if err != nil {
		return event.PublicSearchScope{}, MapError(err)
	}
	return result, nil
}

var _ event.PublicSearchRepository = (*EventRepository)(nil)

func (r *EventRepository) FindPublicDownloadScope(ctx context.Context, token string, now time.Time) (event.PublicDownloadScope, error) {
	var result event.PublicDownloadScope
	err := r.db.QueryRow(ctx, `
		SELECT organization_id, id, downloads_enabled
		FROM events
		WHERE public_token = $1
		  AND visibility = 'public'
		  AND status = 'active'
		  AND (expires_at IS NULL OR expires_at > $2)`, token, now,
	).Scan(&result.OrganizationID, &result.EventID, &result.DownloadsEnabled)
	if err != nil {
		return event.PublicDownloadScope{}, MapError(err)
	}
	return result, nil
}

var _ event.PublicDownloadRepository = (*EventRepository)(nil)

func (r *EventRepository) Status(ctx context.Context, organizationID, eventID string) (event.ProcessingStatus, error) {
	var status event.ProcessingStatus
	status.EventID = eventID
	err := r.db.QueryRow(ctx, `
		SELECT
			count(*) FILTER (WHERE p.status <> 'deleted'),
			count(*) FILTER (WHERE p.status = 'pending'),
			count(*) FILTER (WHERE p.status = 'uploading'),
			count(*) FILTER (WHERE p.status = 'uploaded'),
			count(*) FILTER (WHERE p.status = 'queued'),
			count(*) FILTER (WHERE p.status = 'processing'),
			count(*) FILTER (WHERE p.status = 'ready'),
			count(*) FILTER (WHERE p.status = 'failed'),
			count(*) FILTER (WHERE p.status = 'deleted')
		FROM events e LEFT JOIN photos p
			ON p.organization_id = e.organization_id AND p.event_id = e.id
		WHERE e.organization_id = $1 AND e.id = $2 AND e.status = 'active'
		GROUP BY e.id`, organizationID, eventID,
	).Scan(&status.ActiveTotal, &status.Pending, &status.Uploading, &status.Uploaded,
		&status.Queued, &status.Processing, &status.Ready, &status.Failed, &status.Deleted)
	if err != nil {
		return event.ProcessingStatus{}, MapError(err)
	}
	return status, nil
}

var _ event.Repository = (*EventRepository)(nil)
