// Package outbox implements the transactional outbox pattern.
// Rows are written atomically with domain state changes. A publisher claims
// pending rows, forwards them to Redis, and marks them published or failed.
// A Redis outage leaves committed rows retryable — no committed work is lost.
package outbox

import (
	"context"
	"time"
)

// Message is a single claimed outbox row ready to publish.
type Message struct {
	ID             string
	OrganizationID string
	AggregateType  string
	AggregateID    string
	EventType      string
	Payload        []byte
	IdempotencyKey string
	AttemptCount   int
	CreatedAt      time.Time
}

// Repository provides access to outbox_messages for the publisher loop.
type Repository interface {
	// Claim locks up to limit pending/failed rows that are due and marks them
	// 'publishing'. FOR UPDATE SKIP LOCKED ensures concurrent instances claim
	// disjoint sets.
	Claim(ctx context.Context, limit int) ([]Message, error)

	// MarkPublished transitions a publishing row to 'published'.
	MarkPublished(ctx context.Context, id string) error

	// MarkFailed transitions a publishing row back to 'failed' with an error
	// code and schedules a retry after retryAfter.
	MarkFailed(ctx context.Context, id, errorCode string, retryAfter time.Duration) error

	// RecoverStale resets 'publishing' rows older than leaseTTL back to 'pending'
	// so they are claimed again. Returns the count recovered.
	RecoverStale(ctx context.Context, leaseTTL time.Duration) (int64, error)
}
