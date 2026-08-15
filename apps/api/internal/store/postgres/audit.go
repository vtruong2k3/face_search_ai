package postgres

import (
	"context"
	"encoding/json"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/store"
)

type AuditRepository struct {
	db *Store
}

func NewAuditRepository(db *Store) *AuditRepository {
	return &AuditRepository{db: db}
}

func (r *AuditRepository) WriteAudit(ctx context.Context, record authorization.AuditRecord) error {
	record = authorization.SafeAuditRecord(record)
	if record.Action == "" || record.ResourceType == "" {
		return store.ErrInvalidState
	}
	metadata, err := json.Marshal(record.Metadata)
	if err != nil {
		return store.ErrInvalidState
	}
	_, err = r.db.Exec(ctx, `
		INSERT INTO audit_records(
			organization_id, actor_user_id, action, resource_type,
			resource_id, outcome, request_id, metadata
		) VALUES(NULLIF($1, '')::uuid, NULLIF($2, '')::uuid, $3, $4,
			NULLIF($5, '')::uuid, $6, NULLIF($7, ''), $8::jsonb)`,
		record.OrganizationID, record.ActorUserID, record.Action, record.ResourceType,
		record.ResourceID, record.Outcome, record.RequestID, metadata,
	)
	if err != nil {
		return MapError(err)
	}
	return nil
}

var _ authorization.Auditor = (*AuditRepository)(nil)
