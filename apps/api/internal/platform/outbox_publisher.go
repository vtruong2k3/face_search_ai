package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/face-search-ai/api/internal/domain/outbox"
	"github.com/redis/go-redis/v9"
)

// OutboxPublisherConfig holds bounded outbox publisher settings.
type OutboxPublisherConfig struct {
	StreamName   string
	PollInterval time.Duration
	BatchSize    int
	LeaseTTL     time.Duration
	MaxAttempts  int
}

// DefaultOutboxPublisherConfig returns conservative defaults.
func DefaultOutboxPublisherConfig() OutboxPublisherConfig {
	return OutboxPublisherConfig{
		StreamName:   "photo-jobs",
		PollInterval: 2 * time.Second,
		BatchSize:    50,
		LeaseTTL:     30 * time.Second,
		MaxAttempts:  5,
	}
}

// outboxPublisher claims committed outbox rows and publishes them to Redis Streams.
// Redis unavailability leaves committed rows in 'pending' state — no data loss.
// Deduplication on the Redis side uses a Lua script that XADD only when the
// idempotency key has not been seen; the DB-level UNIQUE constraint and ON CONFLICT
// DO NOTHING already prevent duplicate rows from being created.
type outboxPublisher struct {
	repo   outbox.Repository
	redis  *redis.Client
	cfg    OutboxPublisherConfig
	logger *slog.Logger
}

// luaXADDDedup atomically checks a dedup set and XADDs only if not seen.
// KEYS: [1]=stream, [2]=dedup-set
// ARGV: [1]=idempotency_key, [2..]=XADD field-value pairs
// Returns 1 if published, 0 if duplicate.
var luaXADDDedup = redis.NewScript(`
local key = KEYS[2]
local idem = ARGV[1]
if redis.call('SISMEMBER', key, idem) == 1 then
  return 0
end
local fields = {}
for i = 2, #ARGV do
  fields[#fields+1] = ARGV[i]
end
redis.call('XADD', KEYS[1], '*', unpack(fields))
redis.call('SADD', key, idem)
return 1
`)

// jobEnvelope is the Redis Stream message payload.
type jobEnvelope struct {
	PhotoID              string `json:"photoId"`
	OrganizationID       string `json:"organizationId"`
	EventID              string `json:"eventId"`
	ObjectKey            string `json:"objectKey"`
	ProcessingGeneration int    `json:"processingGeneration"`
	IdempotencyKey       string `json:"idempotencyKey"`
	AttemptCount         int    `json:"attemptCount"`
}

// newOutboxPublisher creates a publisher; call Run to start the polling loop.
func newOutboxPublisher(repo outbox.Repository, redisClient *redis.Client, cfg OutboxPublisherConfig) *outboxPublisher {
	return &outboxPublisher{repo: repo, redis: redisClient, cfg: cfg, logger: slog.Default()}
}

// Run starts the outbox polling loop. It returns when ctx is cancelled.
func (p *outboxPublisher) Run(ctx context.Context) {
	ticker := time.NewTicker(p.cfg.PollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.recoverStale(ctx)
			p.publishBatch(ctx)
		}
	}
}

func (p *outboxPublisher) recoverStale(ctx context.Context) {
	n, err := p.repo.RecoverStale(ctx, p.cfg.LeaseTTL)
	if err != nil {
		p.logger.Error("outbox: stale recovery error", "err", err)
		return
	}
	if n > 0 {
		p.logger.Info("outbox: recovered stale messages", "count", n)
	}
}

func (p *outboxPublisher) publishBatch(ctx context.Context) {
	messages, err := p.repo.Claim(ctx, p.cfg.BatchSize)
	if err != nil {
		p.logger.Error("outbox: claim error", "err", err)
		return
	}
	for _, msg := range messages {
		if err := p.publish(ctx, msg); err != nil {
			retryAfter := backoff(msg.AttemptCount)
			if markErr := p.repo.MarkFailed(ctx, msg.ID, errorCode(err), retryAfter); markErr != nil {
				p.logger.Error("outbox: mark failed error", "err", markErr, "id", msg.ID)
			}
		} else {
			if markErr := p.repo.MarkPublished(ctx, msg.ID); markErr != nil {
				p.logger.Error("outbox: mark published error", "err", markErr, "id", msg.ID)
			}
		}
	}
}

func (p *outboxPublisher) publish(ctx context.Context, msg outbox.Message) error {
	if p.redis == nil {
		return fmt.Errorf("redis client not configured")
	}
	var rawPayload map[string]any
	if err := json.Unmarshal(msg.Payload, &rawPayload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	envelope := jobEnvelope{
		IdempotencyKey: msg.IdempotencyKey,
		AttemptCount:   msg.AttemptCount,
		OrganizationID: msg.OrganizationID,
	}
	if v, ok := rawPayload["photoId"].(string); ok {
		envelope.PhotoID = v
	}
	if v, ok := rawPayload["eventId"].(string); ok {
		envelope.EventID = v
	}
	if v, ok := rawPayload["objectKey"].(string); ok {
		envelope.ObjectKey = v
	}
	if v, ok := rawPayload["processingGeneration"].(float64); ok {
		envelope.ProcessingGeneration = int(v)
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal envelope: %w", err)
	}

	dedupKey := p.cfg.StreamName + ":dedup"
	result, err := luaXADDDedup.Run(ctx, p.redis,
		[]string{p.cfg.StreamName, dedupKey},
		msg.IdempotencyKey,
		"type", msg.EventType,
		"payload", string(payload),
	).Int()
	if err != nil {
		return fmt.Errorf("redis xadd: %w", err)
	}
	if result == 0 {
		// Duplicate — already published. Still mark as published so the row is
		// cleaned up; idempotency is preserved on the worker side.
	}
	return nil
}

// backoff returns an exponential-ish retry delay capped at 5 minutes.
func backoff(attempts int) time.Duration {
	const base = 5 * time.Second
	d := base * (1 << min(attempts, 6))
	if d > 5*time.Minute {
		return 5 * time.Minute
	}
	return d
}

func errorCode(err error) string {
	if err == nil {
		return ""
	}
	if len(err.Error()) > 64 {
		return err.Error()[:64]
	}
	return err.Error()
}
