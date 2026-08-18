package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/face-search-ai/api/internal/store"
)

// TestIntegrationPhotoDeletionSchedulesPurge proves, against a real database,
// that deleting a photo tombstones it, neutralizes its pending processing outbox
// message, and writes exactly one idempotent 'photo.deletion.requested' purge
// message — and that re-deleting causes no error and no duplicate purge.
func TestIntegrationPhotoDeletionSchedulesPurge(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 4, 2)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	userID, orgID, eventID := seedTenant(t, ctx, db, "photo-del@example.test", "photo-del-org")
	photoID := seedReadyPhoto(t, ctx, db, orgID, eventID, userID, "del")

	// A pending processing message that must be neutralized on delete.
	if _, err := db.Exec(ctx, `
		INSERT INTO outbox_messages (organization_id, aggregate_type, aggregate_id, event_type, payload, idempotency_key)
		VALUES ($1, 'photo', $2, 'photo.processing.requested', '{}'::jsonb, $3)`,
		orgID, photoID, "photo.process:"+photoID+":0"); err != nil {
		t.Fatalf("seed processing outbox: %v", err)
	}

	repo := NewPhotoRepository(db)
	if err := repo.Delete(ctx, orgID, eventID, photoID); err != nil {
		t.Fatalf("delete: %v", err)
	}

	var status string
	if err := db.QueryRow(ctx, `SELECT status FROM photos WHERE id = $1`, photoID).Scan(&status); err != nil || status != "deleted" {
		t.Fatalf("photo not tombstoned: status=%q err=%v", status, err)
	}
	var processingStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM outbox_messages WHERE organization_id=$1 AND aggregate_id=$2 AND event_type='photo.processing.requested'`, orgID, photoID).Scan(&processingStatus); err != nil {
		t.Fatalf("read processing outbox: %v", err)
	}
	if processingStatus != "published" {
		t.Fatalf("pending processing message not neutralized: %q", processingStatus)
	}

	countPurge := func() int {
		var n int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_messages WHERE organization_id=$1 AND aggregate_id=$2 AND event_type='photo.deletion.requested'`, orgID, photoID).Scan(&n); err != nil {
			t.Fatalf("count purge: %v", err)
		}
		return n
	}
	if countPurge() != 1 {
		t.Fatalf("expected one purge message, got %d", countPurge())
	}

	// Idempotent: re-deleting must not error or duplicate the purge message.
	if err := repo.Delete(ctx, orgID, eventID, photoID); err != nil {
		t.Fatalf("re-delete: %v", err)
	}
	if countPurge() != 1 {
		t.Fatalf("re-delete duplicated purge message: %d", countPurge())
	}
}

// TestIntegrationEventArchiveSchedulesPurge proves that archiving an event
// tombstones it, neutralizes its photos' pending processing work, and writes
// exactly one idempotent 'event.deletion.requested' purge message.
func TestIntegrationEventArchiveSchedulesPurge(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 4, 2)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	userID, orgID, eventID := seedTenant(t, ctx, db, "event-del@example.test", "event-del-org")
	photoID := seedReadyPhoto(t, ctx, db, orgID, eventID, userID, "evt")
	if _, err := db.Exec(ctx, `
		INSERT INTO outbox_messages (organization_id, aggregate_type, aggregate_id, event_type, payload, idempotency_key)
		VALUES ($1, 'photo', $2, 'photo.processing.requested', '{}'::jsonb, $3)`,
		orgID, photoID, "photo.process:"+photoID+":0"); err != nil {
		t.Fatalf("seed processing outbox: %v", err)
	}

	repo := NewEventRepository(db)
	if err := repo.Archive(ctx, orgID, eventID); err != nil {
		t.Fatalf("archive: %v", err)
	}

	var eventStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM events WHERE id=$1`, eventID).Scan(&eventStatus); err != nil || eventStatus != "archived" {
		t.Fatalf("event not archived: %q err=%v", eventStatus, err)
	}
	var processingStatus string
	if err := db.QueryRow(ctx, `SELECT status FROM outbox_messages WHERE organization_id=$1 AND aggregate_id=$2 AND event_type='photo.processing.requested'`, orgID, photoID).Scan(&processingStatus); err != nil || processingStatus != "published" {
		t.Fatalf("photo processing message not neutralized: %q err=%v", processingStatus, err)
	}

	var purgeCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM outbox_messages WHERE organization_id=$1 AND aggregate_id=$2 AND event_type='event.deletion.requested'`, orgID, eventID).Scan(&purgeCount); err != nil || purgeCount != 1 {
		t.Fatalf("expected one event purge message, got %d err=%v", purgeCount, err)
	}

	// Re-archiving an already-archived event is rejected uniformly.
	if err := repo.Archive(ctx, orgID, eventID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("re-archive should be ErrNotFound, got %v", err)
	}
}

// TestIntegrationDeletedPhotoNeitherSearchableNorDownloadable proves the core
// privacy guarantee at the database boundary: once a photo is tombstoned, the
// download resolver refuses it and the search visibility filter drops it, and
// once its Event is archived the same holds for every photo in it.
func TestIntegrationDeletedPhotoNeitherSearchableNorDownloadable(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 4, 2)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	userID, orgID, eventID := seedTenant(t, ctx, db, "guarantee@example.test", "guarantee-org")
	photoID := seedReadyPhoto(t, ctx, db, orgID, eventID, userID, "guard")

	downloads := NewDownloadRepository(db)
	search := NewSearchRepository(db)

	// While ready: downloadable and search-visible.
	if _, err := downloads.FindDownloadable(ctx, orgID, eventID, photoID); err != nil {
		t.Fatalf("ready photo should be downloadable: %v", err)
	}
	visible, err := search.FilterVisiblePhotoIDs(ctx, orgID, eventID, []string{photoID})
	if err != nil || len(visible) != 1 {
		t.Fatalf("ready photo should be visible: %v %#v", err, visible)
	}

	// After delete: neither downloadable nor visible.
	if err := NewPhotoRepository(db).Delete(ctx, orgID, eventID, photoID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := downloads.FindDownloadable(ctx, orgID, eventID, photoID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("deleted photo must not be downloadable, got %v", err)
	}
	visible, err = search.FilterVisiblePhotoIDs(ctx, orgID, eventID, []string{photoID})
	if err != nil || len(visible) != 0 {
		t.Fatalf("deleted photo must not be search-visible: %v %#v", err, visible)
	}

	// A second photo whose Event gets archived is likewise removed from both.
	other := seedReadyPhoto(t, ctx, db, orgID, eventID, userID, "guard2")
	if err := NewEventRepository(db).Archive(ctx, orgID, eventID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, err := downloads.FindDownloadable(ctx, orgID, eventID, other); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("photo in archived event must not be downloadable, got %v", err)
	}
	visible, err = search.FilterVisiblePhotoIDs(ctx, orgID, eventID, []string{other})
	if err != nil || len(visible) != 0 {
		t.Fatalf("photo in archived event must not be search-visible: %v %#v", err, visible)
	}
}

func seedTenant(t *testing.T, ctx context.Context, db *Store, email, slug string) (userID, orgID, eventID string) {
	t.Helper()
	if err := db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'hash') RETURNING id`, email).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ($1, $2) RETURNING id`, slug, slug).Scan(&orgID); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO events (organization_id, name, visibility, downloads_enabled, created_by_user_id) VALUES ($1, 'Event', 'public', true, $2) RETURNING id`, orgID, userID).Scan(&eventID); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	return userID, orgID, eventID
}

func seedReadyPhoto(t *testing.T, ctx context.Context, db *Store, orgID, eventID, userID, tag string) string {
	t.Helper()
	var id string
	key := "organizations/" + orgID + "/events/" + eventID + "/photos/" + tag + "/original"
	if err := db.QueryRow(ctx, `
		INSERT INTO photos (organization_id, event_id, object_key, content_type, byte_size, status, created_by_user_id)
		VALUES ($1, $2, $3, 'image/jpeg', 1024, 'ready', $4) RETURNING id`,
		orgID, eventID, key, userID).Scan(&id); err != nil {
		t.Fatalf("seed photo: %v", err)
	}
	return id
}
