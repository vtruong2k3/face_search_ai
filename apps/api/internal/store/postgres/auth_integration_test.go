package postgres

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/store"
)

func TestIntegrationAuthRepositoryLifecycle(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 4, 1)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	repo := NewAuthRepository(db)

	passwordHash, err := auth.HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	firstRaw := "first-test-refresh-token"
	user, err := repo.CreateUserWithSession(ctx, "auth-repository@example.test", passwordHash, auth.RefreshHash(firstRaw), time.Now().Add(time.Hour))
	if err != nil {
		t.Fatalf("create user with session: %v", err)
	}

	var storedPassword, storedRefresh string
	if err := db.QueryRow(ctx, "SELECT u.password_hash,s.refresh_token_hash FROM users u JOIN auth_sessions s ON s.user_id=u.id WHERE u.id=$1", user.ID).Scan(&storedPassword, &storedRefresh); err != nil {
		t.Fatalf("read stored credentials: %v", err)
	}
	if storedPassword == "correct-horse-battery" || !auth.VerifyPassword(storedPassword, "correct-horse-battery") {
		t.Fatal("password was not stored as an Argon2id hash")
	}
	if storedRefresh == firstRaw || storedRefresh != auth.RefreshHash(firstRaw) {
		t.Fatal("refresh token was not stored as a hash")
	}

	_, err = repo.CreateUserWithSession(ctx, "auth-repository@example.test", passwordHash, auth.RefreshHash("duplicate-session"), time.Now().Add(time.Hour))
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("duplicate email error = %v", err)
	}
	var sessionCount int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM auth_sessions WHERE user_id=$1", user.ID).Scan(&sessionCount); err != nil || sessionCount != 1 {
		t.Fatalf("duplicate registration created a session: count=%d err=%v", sessionCount, err)
	}

	secondRaw := "second-test-refresh-token"
	replacement, err := repo.RotateSession(ctx, auth.RefreshHash(firstRaw), auth.RefreshHash(secondRaw), time.Now().Add(2*time.Hour))
	if err != nil {
		t.Fatalf("rotate session: %v", err)
	}
	var oldStatus, replacementStatus, replacedBy string
	if err := db.QueryRow(ctx, "SELECT status,replaced_by_session_id FROM auth_sessions WHERE refresh_token_hash=$1", auth.RefreshHash(firstRaw)).Scan(&oldStatus, &replacedBy); err != nil {
		t.Fatalf("read old session: %v", err)
	}
	if err := db.QueryRow(ctx, "SELECT status FROM auth_sessions WHERE id=$1", replacement.ID).Scan(&replacementStatus); err != nil {
		t.Fatalf("read replacement session: %v", err)
	}
	if oldStatus != "rotated" || replacedBy != replacement.ID || replacementStatus != "active" {
		t.Fatalf("rotation state old=%q replacedBy=%q replacement=%q", oldStatus, replacedBy, replacementStatus)
	}
	if _, err := repo.RotateSession(ctx, auth.RefreshHash(firstRaw), auth.RefreshHash("replay-replacement"), time.Now().Add(time.Hour)); !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("replay error = %v", err)
	}

	if err := repo.RevokeSession(ctx, auth.RefreshHash(secondRaw)); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	var revokedStatus string
	var revokedAt *time.Time
	if err := db.QueryRow(ctx, "SELECT status,revoked_at FROM auth_sessions WHERE id=$1", replacement.ID).Scan(&revokedStatus, &revokedAt); err != nil || revokedStatus != "revoked" || revokedAt == nil {
		t.Fatalf("revoked state status=%q at=%v err=%v", revokedStatus, revokedAt, err)
	}
}

func TestIntegrationAuthRepositoryRejectsExpiredSession(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var userID string
	if err := db.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,$2) RETURNING id", "expired-auth@example.test", "hash").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	expiredHash := auth.RefreshHash("expired-test-token")
	if _, err := db.Exec(ctx, "INSERT INTO auth_sessions(user_id,refresh_token_hash,family_id,expires_at,created_at) VALUES($1,$2,gen_random_uuid(),now()-interval '1 minute',now()-interval '2 minutes')", userID, expiredHash); err != nil {
		t.Fatal(err)
	}
	_, err = NewAuthRepository(db).RotateSession(ctx, expiredHash, auth.RefreshHash("must-not-exist"), time.Now().Add(time.Hour))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Fatalf("expired rotation error = %v", err)
	}
	var replacementExists bool
	if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM auth_sessions WHERE refresh_token_hash=$1)", auth.RefreshHash("must-not-exist")).Scan(&replacementExists); err != nil || replacementExists {
		t.Fatalf("expired rotation committed replacement=%v err=%v", replacementExists, err)
	}
}

func TestIntegrationAuthErrorsDoNotLeakCredentialValues(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	db, err := Open(context.Background(), databaseURL, 2, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, _, err = NewAuthRepository(db).FindUserByEmail(context.Background(), "missing-sensitive@example.test")
	if !errors.Is(err, store.ErrNotFound) || strings.Contains(err.Error(), "missing-sensitive") {
		t.Fatalf("unsafe missing-user error: %v", err)
	}
}
