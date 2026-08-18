package postgres

import (
	"context"

	"github.com/face-search-ai/api/internal/domain/search"
)

// SearchRepository backs the public search flow's authoritative visibility
// enforcement. It is deliberately separate from the vector index: even if a
// deleted Photo's vectors have not yet been purged from Qdrant, this filter
// removes it from results, so a tombstoned Photo or archived Event is never
// searchable.
type SearchRepository struct{ db *Store }

func NewSearchRepository(db *Store) *SearchRepository { return &SearchRepository{db: db} }

// FilterVisiblePhotoIDs returns the subset of the supplied photo IDs that are
// still publicly visible for the tenant and Event: present, in the READY state,
// and belonging to an active Event. The query is always tenant- and
// Event-scoped, so it can never leak across tenants or Events. Any other photo
// (deleted, failed, still processing, foreign, or unknown) is dropped.
func (r *SearchRepository) FilterVisiblePhotoIDs(ctx context.Context, organizationID, eventID string, photoIDs []string) ([]string, error) {
	if organizationID == "" || eventID == "" || len(photoIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT p.id::text
		FROM photos p JOIN events e
			ON e.organization_id = p.organization_id AND e.id = p.event_id
		WHERE p.organization_id = $1 AND p.event_id = $2
			AND p.status = 'ready' AND e.status = 'active'
			AND p.id = ANY($3::uuid[])`, organizationID, eventID, photoIDs)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	visible := make([]string, 0, len(photoIDs))
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, MapError(err)
		}
		visible = append(visible, id)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	return visible, nil
}

var _ search.VisibilityFilter = (*SearchRepository)(nil)
