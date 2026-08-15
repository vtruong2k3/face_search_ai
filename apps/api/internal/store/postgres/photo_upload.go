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

// FindForCompletion returns the upload session for idempotent completion.
// It matches either an active session (first-time completion) or a completed
// session where the photo is already queued/processing/ready (replay path).
func (r *PhotoUploadRepository) FindForCompletion(ctx context.Context, organizationID, eventID, photoID, uploadID string, now time.Time) (photo.UploadSession, bool, error) {
	result, err := queryUploadSession(ctx, r.db, `
		SELECT s.id, s.organization_id, s.event_id, s.photo_id, s.upload_id,
			p.object_key, coalesce(p.content_type, ''), coalesce(p.byte_size, 0),
			coalesce(p.checksum_sha256, ''), s.status, s.expires_at, s.created_at, s.updated_at
		FROM photo_upload_sessions s
		JOIN photos p ON p.organization_id = s.organization_id AND p.event_id = s.event_id AND p.id = s.photo_id
		JOIN events e ON e.organization_id = p.organization_id AND e.id = p.event_id
		WHERE s.organization_id = $1 AND s.event_id = $2 AND s.photo_id = $3
			AND s.upload_id = $4 AND e.status = 'active'
			AND (
				(s.status = 'active' AND s.expires_at > $5 AND p.status IN ('pending', 'uploading'))
				OR
				(s.status = 'completed' AND p.status IN ('queued', 'processing', 'ready'))
			)`, organizationID, eventID, photoID, uploadID, now)
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
		WITH expired AS (
			UPDATE photo_upload_sessions SET status = 'expired', updated_at = now()
			WHERE organization_id = $1 AND event_id = $2 AND photo_id = $3
				AND status = 'active' AND expires_at <= now()
			RETURNING photo_id
		), trusted AS (
			UPDATE photos p SET status = 'uploading', updated_at = now()
			FROM events e
			WHERE p.organization_id = $1 AND p.event_id = $2 AND p.id = $3
				AND p.status IN ('pending', 'uploading')
				AND e.organization_id = p.organization_id AND e.id = p.event_id AND e.status = 'active'
			RETURNING p.organization_id, p.event_id, p.id
		), expired_gate AS (
			SELECT count(*) AS count FROM expired
		), inserted AS (
			INSERT INTO photo_upload_sessions (organization_id, event_id, photo_id, upload_id, expires_at)
			SELECT organization_id, event_id, id, $4, $5 FROM trusted CROSS JOIN expired_gate
			RETURNING *
		)
		SELECT s.id, s.organization_id, s.event_id, s.photo_id, s.upload_id,
			p.object_key, coalesce(p.content_type, ''), coalesce(p.byte_size, 0),
			coalesce(p.checksum_sha256, ''), s.status, s.expires_at, s.created_at, s.updated_at
		FROM inserted s JOIN photos p ON p.organization_id = s.organization_id AND p.event_id = s.event_id AND p.id = s.photo_id`,
		organizationID, eventID, photoID, uploadID, expiresAt)
}

// FinalizeCompleted atomically:
//  1. Marks the upload session as completed.
//  2. Transitions the photo: uploading → uploaded → queued.
//  3. Inserts a versioned outbox message with ON CONFLICT DO NOTHING (idempotent).
//
// Returns the persisted photo including its updated processing_generation.
func (r *PhotoUploadRepository) FinalizeCompleted(ctx context.Context, organizationID, eventID, photoID, uploadID string) (photo.Photo, error) {
	var result photo.Photo
	err := r.db.WithinTransaction(ctx, func(ctx context.Context, tx store.DBTX) error {
		// Step 1: mark session completed (only if currently active).
		// Scan photo_id to get a no-rows error if the row doesn't exist.
		var markedPhotoID string
		err := tx.QueryRow(ctx, `
			UPDATE photo_upload_sessions SET status = 'completed', completed_at = now(), updated_at = now()
			WHERE organization_id = $1 AND event_id = $2 AND photo_id = $3 AND upload_id = $4 AND status = 'active'
			RETURNING photo_id`,
			organizationID, eventID, photoID, uploadID,
		).Scan(&markedPhotoID)
		if err != nil {
			return MapError(err)
		}
		_ = markedPhotoID

		// Step 2: transition photo uploading → uploaded → queued and increment processing_generation.
		err = tx.QueryRow(ctx, `
			UPDATE photos SET status = 'queued', processing_generation = processing_generation + 1,
				failure_code = NULL, updated_at = now()
			WHERE organization_id = $1 AND event_id = $2 AND id = $3 AND status = 'uploading'
			RETURNING id, organization_id, event_id, object_key, coalesce(original_filename, ''),
				coalesce(content_type, ''), coalesce(byte_size, 0), coalesce(checksum_sha256, ''),
				status, coalesce(failure_code, ''), processing_generation, created_by_user_id, created_at, updated_at`,
			organizationID, eventID, photoID,
		).Scan(&result.ID, &result.OrganizationID, &result.EventID, &result.ObjectKey,
			&result.OriginalFilename, &result.ContentType, &result.ByteSize, &result.ChecksumSHA256,
			&result.Status, &result.FailureCode, &result.ProcessingGeneration,
			&result.CreatedByUserID, &result.CreatedAt, &result.UpdatedAt)
		if err != nil {
			return MapError(err)
		}

		// Step 3: insert outbox message with ON CONFLICT DO NOTHING (idempotent across retries).
		idempotencyKey := result.IdempotencyKey()
		_, err = tx.Exec(ctx, `
			INSERT INTO outbox_messages (
				organization_id, aggregate_type, aggregate_id, event_type,
				payload, idempotency_key
			) VALUES (
				$1, 'photo', $2, 'photo.processing.requested',
				jsonb_build_object(
					'photoId', $2::text,
					'organizationId', $1::text,
					'eventId', $3::text,
					'objectKey', $4::text,
					'processingGeneration', $5::int
				),
				$6
			) ON CONFLICT (organization_id, idempotency_key) DO NOTHING`,
			organizationID, photoID, eventID, result.ObjectKey, result.ProcessingGeneration, idempotencyKey)
		if err != nil {
			return MapError(err)
		}
		return nil
	})
	if err != nil {
		return photo.Photo{}, err
	}
	return result, nil
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
