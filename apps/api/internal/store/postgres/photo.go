package postgres

import (
	"context"

	"github.com/face-search-ai/api/internal/domain/photo"
	"github.com/face-search-ai/api/internal/store"
)

type PhotoRepository struct{ db *Store }

func NewPhotoRepository(db *Store) *PhotoRepository { return &PhotoRepository{db: db} }

func (r *PhotoRepository) Create(ctx context.Context, params photo.CreateParams) (photo.Photo, error) {
	var result photo.Photo
	err := r.db.QueryRow(ctx, `
		WITH trusted AS (
			SELECT gen_random_uuid() AS id
			FROM events
			WHERE organization_id = $1 AND id = $2 AND status = 'active'
		)
		INSERT INTO photos (
			id, organization_id, event_id, object_key, original_filename,
			content_type, byte_size, checksum_sha256, created_by_user_id
		)
		SELECT id, $1, $2,
			'organizations/' || $1::text || '/events/' || $2::text || '/photos/' || id::text || '/original',
			$3, $4, $5, nullif($6, ''), $7
		FROM trusted
		RETURNING id, organization_id, event_id, object_key, original_filename,
			content_type, byte_size, coalesce(checksum_sha256, ''), status,
			coalesce(failure_code, ''), processing_generation, created_by_user_id, created_at, updated_at`,
		params.OrganizationID, params.EventID, params.OriginalFilename, params.ContentType,
		params.ByteSize, params.ChecksumSHA256, params.CreatedByUserID,
	).Scan(&result.ID, &result.OrganizationID, &result.EventID, &result.ObjectKey,
		&result.OriginalFilename, &result.ContentType, &result.ByteSize, &result.ChecksumSHA256,
		&result.Status, &result.FailureCode, &result.ProcessingGeneration,
		&result.CreatedByUserID, &result.CreatedAt, &result.UpdatedAt)
	if err != nil {
		return photo.Photo{}, MapError(err)
	}
	return result, nil
}

func (r *PhotoRepository) List(ctx context.Context, organizationID, eventID string) ([]photo.Photo, error) {
	rows, err := r.db.Query(ctx, `
		SELECT p.id, p.organization_id, p.event_id, p.object_key, coalesce(p.original_filename, ''),
			coalesce(p.content_type, ''), coalesce(p.byte_size, 0), coalesce(p.checksum_sha256, ''),
			p.status, coalesce(p.failure_code, ''), p.processing_generation,
			p.created_by_user_id, p.created_at, p.updated_at
		FROM photos p JOIN events e ON e.organization_id = p.organization_id AND e.id = p.event_id
		WHERE p.organization_id = $1 AND p.event_id = $2 AND p.status <> 'deleted' AND e.status = 'active'
		ORDER BY p.created_at DESC, p.id`, organizationID, eventID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	results := make([]photo.Photo, 0)
	for rows.Next() {
		result, err := scanPhoto(rows)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	return results, nil
}

func (r *PhotoRepository) Find(ctx context.Context, organizationID, eventID, photoID string) (photo.Photo, error) {
	return queryPhoto(ctx, r.db, `
		SELECT p.id, p.organization_id, p.event_id, p.object_key, coalesce(p.original_filename, ''),
			coalesce(p.content_type, ''), coalesce(p.byte_size, 0), coalesce(p.checksum_sha256, ''),
			p.status, coalesce(p.failure_code, ''), p.processing_generation,
			p.created_by_user_id, p.created_at, p.updated_at
		FROM photos p JOIN events e ON e.organization_id = p.organization_id AND e.id = p.event_id
		WHERE p.organization_id = $1 AND p.event_id = $2 AND p.id = $3
			AND p.status <> 'deleted' AND e.status = 'active'`, organizationID, eventID, photoID)
}

// Delete tombstones a photo and durably schedules purge of its underlying data.
// In a single transaction it (1) sets the photo to the terminal 'deleted' state
// — which immediately makes it non-downloadable (downloads require 'ready') and
// non-searchable (the search visibility filter requires 'ready'); (2) neutralizes
// any not-yet-published processing outbox message for the photo so no new work is
// enqueued; and (3) writes a versioned 'photo.deletion.requested' outbox message
// that the worker consumes to purge MinIO objects, Qdrant vectors, and face rows.
// It is idempotent: re-deleting an already-deleted photo re-affirms the tombstone,
// the neutralize is a no-op, and ON CONFLICT DO NOTHING prevents duplicate purge
// messages. It is fail-closed and tenant/Event-scoped; a missing or foreign
// resource yields ErrNotFound.
func (r *PhotoRepository) Delete(ctx context.Context, organizationID, eventID, photoID string) error {
	return r.db.WithinTransaction(ctx, func(ctx context.Context, tx store.DBTX) error {
		var objectKey string
		err := tx.QueryRow(ctx, `
			UPDATE photos p SET status = 'deleted', failure_code = NULL, updated_at = now()
			FROM events e
			WHERE p.organization_id = $1 AND p.event_id = $2 AND p.id = $3
				AND e.organization_id = p.organization_id AND e.id = p.event_id AND e.status = 'active'
			RETURNING p.object_key`,
			organizationID, eventID, photoID).Scan(&objectKey)
		if err != nil {
			return MapError(err)
		}
		if _, err := tx.Exec(ctx, `
			UPDATE outbox_messages
			SET status = 'published', published_at = now(), updated_at = now()
			WHERE organization_id = $1 AND aggregate_type = 'photo' AND aggregate_id = $2
				AND event_type = 'photo.processing.requested' AND status IN ('pending', 'failed')`,
			organizationID, photoID); err != nil {
			return MapError(err)
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO outbox_messages (
				organization_id, aggregate_type, aggregate_id, event_type,
				payload, idempotency_key
			) VALUES (
				$1, 'photo', $2, 'photo.deletion.requested',
				jsonb_build_object(
					'photoId', $2::text,
					'organizationId', $1::text,
					'eventId', $3::text,
					'objectKey', $4::text
				),
				$5
			) ON CONFLICT (organization_id, idempotency_key) DO NOTHING`,
			organizationID, photoID, eventID, objectKey, "photo.delete:"+photoID); err != nil {
			return MapError(err)
		}
		return nil
	})
}

// Reprocess atomically transitions a failed photo to queued, increments processing_generation,
// and inserts a versioned outbox message. ON CONFLICT DO NOTHING makes it idempotent.
func (r *PhotoRepository) Reprocess(ctx context.Context, organizationID, eventID, photoID string) (photo.Photo, error) {
	var result photo.Photo
	err := r.db.WithinTransaction(ctx, func(ctx context.Context, tx store.DBTX) error {
		err := tx.QueryRow(ctx, `
			UPDATE photos p SET status = 'queued', failure_code = NULL,
				processing_generation = processing_generation + 1, updated_at = now()
			FROM events e
			WHERE p.organization_id = $1 AND p.event_id = $2 AND p.id = $3 AND p.status = 'failed'
				AND e.organization_id = p.organization_id AND e.id = p.event_id AND e.status = 'active'
			RETURNING p.id, p.organization_id, p.event_id, p.object_key, coalesce(p.original_filename, ''),
				coalesce(p.content_type, ''), coalesce(p.byte_size, 0), coalesce(p.checksum_sha256, ''),
				p.status, coalesce(p.failure_code, ''), p.processing_generation,
				p.created_by_user_id, p.created_at, p.updated_at`,
			organizationID, eventID, photoID,
		).Scan(&result.ID, &result.OrganizationID, &result.EventID, &result.ObjectKey,
			&result.OriginalFilename, &result.ContentType, &result.ByteSize, &result.ChecksumSHA256,
			&result.Status, &result.FailureCode, &result.ProcessingGeneration,
			&result.CreatedByUserID, &result.CreatedAt, &result.UpdatedAt)
		if err != nil {
			return MapError(err)
		}
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

type photoScanner interface{ Scan(...any) error }

func scanPhoto(row photoScanner) (photo.Photo, error) {
	var result photo.Photo
	if err := row.Scan(&result.ID, &result.OrganizationID, &result.EventID, &result.ObjectKey,
		&result.OriginalFilename, &result.ContentType, &result.ByteSize, &result.ChecksumSHA256,
		&result.Status, &result.FailureCode, &result.ProcessingGeneration,
		&result.CreatedByUserID, &result.CreatedAt, &result.UpdatedAt); err != nil {
		return photo.Photo{}, MapError(err)
	}
	return result, nil
}

func queryPhoto(ctx context.Context, db store.DBTX, query string, args ...any) (photo.Photo, error) {
	return scanPhoto(db.QueryRow(ctx, query, args...))
}

var _ photo.Repository = (*PhotoRepository)(nil)
