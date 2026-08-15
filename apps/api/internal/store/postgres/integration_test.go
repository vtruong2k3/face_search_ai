package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/face-search-ai/api/internal/store"
)

func TestIntegrationTransactionsAndSchema(t *testing.T) {
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
	if err := db.Ping(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}
	if err := db.CheckSchema(ctx); err != nil {
		t.Fatalf("schema: %v", err)
	}

	var committedID string
	err = db.WithinTransaction(ctx, func(ctx context.Context, tx store.DBTX) error {
		return tx.QueryRow(ctx, `INSERT INTO users (email, password_hash) VALUES ('commit@example.test', 'hash') RETURNING id`).Scan(&committedID)
	})
	if err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	var committed bool
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)", committedID).Scan(&committed); err != nil || !committed {
		t.Fatalf("committed row missing: exists=%v err=%v", committed, err)
	}

	sentinel := errors.New("stop transaction")
	err = db.WithinTransaction(ctx, func(ctx context.Context, tx store.DBTX) error {
		if _, err := tx.Exec(ctx, `INSERT INTO users (email, password_hash) VALUES ('rollback@example.test', 'hash')`); err != nil {
			return err
		}
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("rollback error = %v", err)
	}
	var rolledBack bool
	if err := db.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM users WHERE email = 'rollback@example.test')").Scan(&rolledBack); err != nil || rolledBack {
		t.Fatalf("rollback row exists=%v err=%v", rolledBack, err)
	}

	_, err = db.Exec(ctx, `INSERT INTO users (email, password_hash) VALUES ('commit@example.test', 'hash')`)
	err = MapError(err)
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("constraint error = %v", err)
	}
	if strings.Contains(err.Error(), "commit@example.test") {
		t.Fatalf("constraint error leaked data: %v", err)
	}
}

func TestIntegrationSchemaVersionMismatchIsSanitized(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	db, err := Open(context.Background(), databaseURL, 2, 999)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	err = db.CheckSchema(context.Background())
	if !errors.Is(err, store.ErrInvalidState) {
		t.Fatalf("schema error = %v", err)
	}
	if strings.Contains(err.Error(), databaseURL) {
		t.Fatalf("schema error leaked URL: %v", err)
	}
}
