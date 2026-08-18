package postgres

import (
	"context"

	"github.com/face-search-ai/api/internal/domain/download"
	"github.com/face-search-ai/api/internal/store"
)

type DownloadRepository struct{ db *Store }

func NewDownloadRepository(db *Store) *DownloadRepository { return &DownloadRepository{db: db} }

// FindDownloadable enforces result scope in SQL: the photo must belong to the
// exact organization and Event, its Event must still be active, and the photo
// must be in the READY state. Any other case returns store.ErrNotFound, which
// the download service maps to a uniform, non-enumerating rejection.
func (r *DownloadRepository) FindDownloadable(ctx context.Context, organizationID, eventID, photoID string) (download.DownloadableObject, error) {
	var result download.DownloadableObject
	err := r.db.QueryRow(ctx, `
		SELECT p.object_key, coalesce(p.content_type, '')
		FROM photos p JOIN events e
			ON e.organization_id = p.organization_id AND e.id = p.event_id
		WHERE p.organization_id = $1 AND p.event_id = $2 AND p.id = $3
			AND p.status = 'ready' AND e.status = 'active'`, organizationID, eventID, photoID,
	).Scan(&result.ObjectKey, &result.ContentType)
	if err != nil {
		return download.DownloadableObject{}, MapError(err)
	}
	return result, nil
}

var _ download.PhotoResolver = (*DownloadRepository)(nil)

// Record persists a safe, decision-level download audit row. It stores only the
// tenant/Event/photo scope, request kind, decision, and a low-cardinality denial
// code. Signed URLs, object paths, and tokens are never passed in or stored.
func (r *DownloadRepository) Record(ctx context.Context, entry download.AuditEntry) error {
	if entry.OrganizationID == "" || entry.EventID == "" {
		return store.ErrInvalidState
	}
	_, err := r.db.Exec(ctx, `
		INSERT INTO download_records (
			organization_id, event_id, photo_id, kind, decision, denial_code
		) VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, NULLIF($6, ''))`,
		entry.OrganizationID, entry.EventID, entry.PhotoID,
		string(entry.Kind), string(entry.Decision), entry.DenialCode,
	)
	if err != nil {
		return MapError(err)
	}
	return nil
}

var _ download.Recorder = (*DownloadRepository)(nil)
