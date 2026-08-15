package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/face-search-ai/api/internal/domain/photo"
	"github.com/face-search-ai/api/internal/store"
	"github.com/jackc/pgx/v5"
)

type PhotoUploadRepository struct{ db *Store }

func NewPhotoUploadRepository(db *Store) *PhotoUploadRepository {
	return &PhotoUploadRepository{db: db}
}

func (r *PhotoUploadRepository) FindActive(ctx context.Context, organizationID, eventID, photoID string, now time.Time) (photo.UploadSession, bool, error) {
	result, err := queryUploadSession(ctx, r.db, `
		SELECT s.id, s.organization_id, s.event_id, s.photo_id, s.upload_id,
			p.object_key, coalesce(p.content_type, ''), coalesce(p.byte_size, 0),
			coalesce(p.checksum_sha256, ''), s.status, s.expires_at, s.created_at, s.updated_at
		FROM photo_upload_sessions s
		JOIN photos p ON p.organization_id = s.organization_id AND p.event_id = s.event_id AND p.id = s.photo_id
		JOIN events e ON e.organization_id = p.organization_id AND e.id = p.event_id
		WHERE s.organization_id = $1 AND s.event_id = $2 AND s.photo_id = $3
			AND s.status = 'active' AND s.expires_at > $4
			AND p.status IN ('pending', 'uploading') AND e.status = 'active'`, organizationID, eventID, photoID, now)
	if errors.Is(err, pgx.ErrNoRows) {
		return photo.UploadSession{}, false, nil
	}
	if err != nil {
		return photo.UploadSession{}, false, err
	}
	return result, true, nil
}

func (r *PhotoUploadRepository) Create(ctx context.Context, organizationID, eventID, photoID, uploadID string, expiresAt time.Time) (photo.UploadSession, error) {
	return queryUploadSession(ctx, r.db, `
		WITH trusted AS (
			UPDATE photos p SET status = 'uploading', updated_at = now()
			FROM events e
			WHERE p.organization_id = $1 AND p.event_id = $2 AND p.id = $3
				AND p.status IN ('pending', 'uploading')
				AND e.organization_id = p.organization_id AND e.id = p.event_id AND e.status = 'active'
			RETURNING p.organization_id, p.event_id, p.id
		), inserted AS (
			INSERT INTO photo_upload_sessions (organization_id, event_id, photo_id, upload_id, expires_at)
			SELECT organization_id, event_id, id, $4, $5 FROM trusted
			RETURNING *
		)
		SELECT s.id, s.organization_id, s.event_id, s.photo_id, s.upload_id,
			p.object_key, coalesce(p.content_type, ''), coalesce(p.byte_size, 0),
			coalesce(p.checksum_sha256, ''), s.status, s.expires_at, s.created_at, s.updated_at
		FROM inserted s JOIN photos p ON p.organization_id = s.organization_id AND p.event_id = s.event_id AND p.id = s.photo_id`,
		organizationID, eventID, photoID, uploadID, expiresAt)
}

func (r *PhotoUploadRepository) MarkCompleted(ctx context.Context, organizationID, eventID, photoID, uploadID string) error {
	tag, err := r.db.Exec(ctx, `
		WITH completed AS (
			UPDATE photo_upload_sessions SET status = 'completed', completed_at = now(), updated_at = now()
			WHERE organization_id = $1 AND event_id = $2 AND photo_id = $3 AND upload_id = $4 AND status = 'active'
			RETURNING organization_id, event_id, photo_id
		)
		UPDATE photos p SET status = 'uploaded', updated_at = now()
		FROM completed c
		WHERE p.organization_id = c.organization_id AND p.event_id = c.event_id AND p.id = c.photo_id AND p.status = 'uploading'`,
		organizationID, eventID, photoID, uploadID)
	if err != nil {
		return MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

func (r *PhotoUploadRepository) MarkAborted(ctx context.Context, organizationID, eventID, photoID, uploadID string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE photo_upload_sessions SET status = 'aborted', aborted_at = now(), updated_at = now()
		WHERE organization_id = $1 AND event_id = $2 AND photo_id = $3 AND upload_id = $4 AND status = 'active'`,
		organizationID, eventID, photoID, uploadID)
	if err != nil {
		return MapError(err)
	}
	if tag.RowsAffected() == 0 {
		return store.ErrNotFound
	}
	return nil
}

type uploadSessionScanner interface{ Scan(...any) error }

func scanUploadSession(row uploadSessionScanner) (photo.UploadSession, error) {
	var result photo.UploadSession
	if err := row.Scan(&result.ID, &result.OrganizationID, &result.EventID, &result.PhotoID,
		&result.UploadID, &result.ObjectKey, &result.ContentType, &result.ByteSize,
		&result.ChecksumSHA256, &result.Status, &result.ExpiresAt, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return photo.UploadSession{}, MapError(err)
	}
	return result, nil
}

func queryUploadSession(ctx context.Context, db store.DBTX, query string, args ...any) (photo.UploadSession, error) {
	return scanUploadSession(db.QueryRow(ctx, query, args...))
}

var _ photo.UploadSessionRepository = (*PhotoUploadRepository)(nil)
