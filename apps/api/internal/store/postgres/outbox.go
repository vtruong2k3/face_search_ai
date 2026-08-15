package postgres

import (
	"context"
	"time"

	"github.com/face-search-ai/api/internal/domain/outbox"
)

// OutboxRepository provides access to outbox_messages.
type OutboxRepository struct{ db *Store }

func NewOutboxRepository(db *Store) *OutboxRepository { return &OutboxRepository{db: db} }

// Claim locks up to limit due pending/failed rows and transitions them to
// 'publishing'. Callers must call MarkPublished or MarkFailed when done.
// FOR UPDATE SKIP LOCKED lets concurrent callers claim disjoint sets.
func (r *OutboxRepository) Claim(ctx context.Context, limit int) ([]outbox.Message, error) {
	rows, err := r.db.Query(ctx, `
		UPDATE outbox_messages
		SET status = 'publishing', attempt_count = attempt_count + 1, updated_at = now()
		WHERE id IN (
			SELECT id FROM outbox_messages
			WHERE status IN ('pending', 'failed') AND available_at <= now()
			ORDER BY available_at, created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, organization_id, aggregate_type, aggregate_id::text, event_type,
			payload::text, idempotency_key, attempt_count, created_at`,
		limit)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()
	results := make([]outbox.Message, 0, limit)
	for rows.Next() {
		var m outbox.Message
		var payload string
		if err := rows.Scan(&m.ID, &m.OrganizationID, &m.AggregateType, &m.AggregateID,
			&m.EventType, &payload, &m.IdempotencyKey, &m.AttemptCount, &m.CreatedAt); err != nil {
			return nil, MapError(err)
		}
		m.Payload = []byte(payload)
		results = append(results, m)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	return results, nil
}

// MarkPublished transitions a publishing row to 'published'.
func (r *OutboxRepository) MarkPublished(ctx context.Context, id string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE outbox_messages SET status = 'published', published_at = now(), updated_at = now()
		WHERE id = $1 AND status = 'publishing'`, id)
	return MapError(err)
}

// MarkFailed transitions a publishing row back to 'failed' and schedules retry.
func (r *OutboxRepository) MarkFailed(ctx context.Context, id, errorCode string, retryAfter time.Duration) error {
	_, err := r.db.Exec(ctx, `
		UPDATE outbox_messages
		SET status = 'failed', last_error_code = $2,
			available_at = now() + $3::interval, updated_at = now()
		WHERE id = $1 AND status = 'publishing'`,
		id, errorCode, retryAfter.String())
	return MapError(err)
}

// RecoverStale resets publishing rows that have been held past leaseTTL back to
// pending so they are claimed again on the next poll.
func (r *OutboxRepository) RecoverStale(ctx context.Context, leaseTTL time.Duration) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE outbox_messages SET status = 'pending', updated_at = now()
		WHERE status = 'publishing' AND updated_at < now() - $1::interval`,
		leaseTTL.String())
	if err != nil {
		return 0, MapError(err)
	}
	return tag.RowsAffected(), nil
}

var _ outbox.Repository = (*OutboxRepository)(nil)
