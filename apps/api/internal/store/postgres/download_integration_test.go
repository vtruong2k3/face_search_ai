package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/face-search-ai/api/internal/domain/download"
	"github.com/face-search-ai/api/internal/store"
)

// TestIntegrationDownloadRepository proves, against a real database, that
// FindDownloadable binds result scope to the exact organization+Event and the
// READY state, and that Record persists a safe decision-level audit row.
func TestIntegrationDownloadRepository(t *testing.T) {
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

	var userID, orgID, eventID, otherEventID string
	if err := db.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('downloader@example.test', 'hash') RETURNING id`).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO organizations (name, slug) VALUES ('Download Org', 'download-org') RETURNING id`).Scan(&orgID); err != nil {
		t.Fatalf("seed org: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO events (organization_id, name, visibility, downloads_enabled, created_by_user_id) VALUES ($1, 'Event A', 'public', true, $2) RETURNING id`, orgID, userID).Scan(&eventID); err != nil {
		t.Fatalf("seed event: %v", err)
	}
	if err := db.QueryRow(ctx, `INSERT INTO events (organization_id, name, visibility, downloads_enabled, created_by_user_id) VALUES ($1, 'Event B', 'public', true, $2) RETURNING id`, orgID, userID).Scan(&otherEventID); err != nil {
		t.Fatalf("seed other event: %v", err)
	}

	seedPhoto := func(eventID, key, status string) string {
		var id string
		if err := db.QueryRow(ctx, `
			INSERT INTO photos (organization_id, event_id, object_key, content_type, byte_size, status, created_by_user_id)
			VALUES ($1, $2, $3, 'image/jpeg', 1024, $4, $5) RETURNING id`,
			orgID, eventID, key, status, userID).Scan(&id); err != nil {
			t.Fatalf("seed photo: %v", err)
		}
		return id
	}
	readyID := seedPhoto(eventID, "organizations/"+orgID+"/events/"+eventID+"/photos/ready/original", "ready")
	pendingID := seedPhoto(eventID, "organizations/"+orgID+"/events/"+eventID+"/photos/pending/original", "pending")
	otherEventPhotoID := seedPhoto(otherEventID, "organizations/"+orgID+"/events/"+otherEventID+"/photos/x/original", "ready")

	repo := NewDownloadRepository(db)

	object, err := repo.FindDownloadable(ctx, orgID, eventID, readyID)
	if err != nil || object.ObjectKey == "" || object.ContentType != "image/jpeg" {
		t.Fatalf("ready photo should be downloadable: %#v err=%v", object, err)
	}

	if _, err := repo.FindDownloadable(ctx, orgID, eventID, pendingID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("non-ready photo must not be downloadable, got %v", err)
	}
	if _, err := repo.FindDownloadable(ctx, orgID, eventID, otherEventPhotoID); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-Event photo must not be downloadable, got %v", err)
	}

	if err := repo.Record(ctx, download.AuditEntry{OrganizationID: orgID, EventID: eventID, PhotoID: readyID, Kind: download.KindSingle, Decision: download.DecisionAllowed}); err != nil {
		t.Fatalf("record allowed: %v", err)
	}
	if err := repo.Record(ctx, download.AuditEntry{OrganizationID: orgID, EventID: eventID, Kind: download.KindBulk, Decision: download.DecisionDenied, DenialCode: download.DenialDownloadsDisabled}); err != nil {
		t.Fatalf("record denied: %v", err)
	}
	var count int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM download_records WHERE organization_id = $1 AND event_id = $2`, orgID, eventID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("expected two download records, got %d err=%v", count, err)
	}
}
