package platform

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/outbox"
)

// fakeOutboxRepo is an in-memory stub for testing the publisher loop.
type fakeOutboxRepo struct {
	messages  []outbox.Message
	published []string
	failed    []string
	recovered int64
	claimErr  error
}

func (r *fakeOutboxRepo) Claim(_ context.Context, limit int) ([]outbox.Message, error) {
	if r.claimErr != nil {
		return nil, r.claimErr
	}
	n := min(len(r.messages), limit)
	out := r.messages[:n]
	r.messages = r.messages[n:]
	return out, nil
}

func (r *fakeOutboxRepo) MarkPublished(_ context.Context, id string) error {
	r.published = append(r.published, id)
	return nil
}

func (r *fakeOutboxRepo) MarkFailed(_ context.Context, id, _ string, _ time.Duration) error {
	r.failed = append(r.failed, id)
	return nil
}

func (r *fakeOutboxRepo) RecoverStale(_ context.Context, _ time.Duration) (int64, error) {
	r.recovered++
	return 0, nil
}

func makeTestMessage(id, idempotencyKey string, gen int) outbox.Message {
	payload, _ := json.Marshal(map[string]any{
		"photoId":              "photo-1",
		"organizationId":       "org-1",
		"eventId":              "event-1",
		"objectKey":            "organizations/org-1/events/event-1/photos/photo-1/original",
		"processingGeneration": float64(gen),
	})
	return outbox.Message{
		ID:             id,
		OrganizationID: "org-1",
		AggregateType:  "photo",
		AggregateID:    "photo-1",
		EventType:      "photo.processing.requested",
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
		AttemptCount:   1,
		CreatedAt:      time.Now(),
	}
}

func TestPublisherBackoffGrowsWithAttempts(t *testing.T) {
	prev := backoff(0)
	for i := 1; i <= 6; i++ {
		next := backoff(i)
		if next <= prev {
			t.Fatalf("backoff(%d)=%v is not greater than backoff(%d)=%v", i, next, i-1, prev)
		}
		prev = next
	}
	// Capped at 5 minutes
	if cap := backoff(100); cap > 5*time.Minute {
		t.Fatalf("backoff(100)=%v exceeds cap", cap)
	}
}

func TestErrorCodeTruncates(t *testing.T) {
	long := ""
	for range 100 {
		long += "x"
	}
	if got := errorCode(fmt.Errorf("%s", long)); len(got) != 64 {
		t.Fatalf("errorCode length = %d, want 64", len(got))
	}
	if got := errorCode(nil); got != "" {
		t.Fatalf("errorCode(nil) = %q", got)
	}
}

func TestPublisherPublishBatchMarksPublished(t *testing.T) {
	// With a nil Redis client publish() will fail; verify MarkFailed is called.
	repo := &fakeOutboxRepo{messages: []outbox.Message{
		makeTestMessage("msg-1", "photo.process:photo-1:1", 1),
	}}
	cfg := DefaultOutboxPublisherConfig()
	p := newOutboxPublisher(repo, nil, cfg) // redis is nil → publish returns error
	p.publishBatch(context.Background())
	// The message was claimed (removed from repo.messages) and marked failed.
	if len(repo.messages) != 0 {
		t.Fatalf("unclaimed messages = %d", len(repo.messages))
	}
	if len(repo.failed) != 1 {
		t.Fatalf("failed count = %d, want 1", len(repo.failed))
	}
}

func TestPublisherClaimErrorIsNonFatal(t *testing.T) {
	repo := &fakeOutboxRepo{claimErr: fmt.Errorf("db down")}
	cfg := DefaultOutboxPublisherConfig()
	p := newOutboxPublisher(repo, nil, cfg)
	// Should not panic; just logs and returns.
	p.publishBatch(context.Background())
	// No messages were marked published or failed.
	if len(repo.published) != 0 || len(repo.failed) != 0 {
		t.Fatalf("published=%v failed=%v", repo.published, repo.failed)
	}
}

func TestPublisherRecoverStaleIsCalled(t *testing.T) {
	repo := &fakeOutboxRepo{}
	cfg := DefaultOutboxPublisherConfig()
	p := newOutboxPublisher(repo, nil, cfg)
	p.recoverStale(context.Background())
	if repo.recovered != 1 {
		t.Fatalf("recovered = %d", repo.recovered)
	}
}
