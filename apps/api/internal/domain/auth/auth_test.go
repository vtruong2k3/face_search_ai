package auth

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeRepository struct {
	user         User
	passwordHash string
	refreshHash  string
	rotatedHash  string
	revokedHash  string
}

func (f *fakeRepository) CreateUserWithSession(_ context.Context, email, passwordHash, refreshHash string, _ time.Time) (User, error) {
	f.user = User{ID: "user-1", Email: email, Status: "active", CreatedAt: time.Unix(1, 0)}
	f.passwordHash, f.refreshHash = passwordHash, refreshHash
	return f.user, nil
}
func (f *fakeRepository) FindUserByEmail(_ context.Context, email string) (User, string, error) {
	if email != f.user.Email {
		return User{}, "", ErrInvalidCredentials
	}
	return f.user, f.passwordHash, nil
}
func (f *fakeRepository) FindUserByID(_ context.Context, id string) (User, error) {
	if id != f.user.ID {
		return User{}, ErrInvalidCredentials
	}
	return f.user, nil
}
func (f *fakeRepository) CreateSession(_ context.Context, userID, hash string, expires time.Time) (Session, error) {
	f.refreshHash = hash
	return Session{ID: "session-1", UserID: userID, FamilyID: "family-1", Status: "active", ExpiresAt: expires}, nil
}
func (f *fakeRepository) RotateSession(_ context.Context, oldHash, newHash string, expires time.Time) (Session, error) {
	if oldHash != f.refreshHash {
		return Session{}, ErrInvalidCredentials
	}
	f.rotatedHash, f.refreshHash = oldHash, newHash
	return Session{ID: "session-2", UserID: f.user.ID, FamilyID: "family-1", Status: "active", ExpiresAt: expires}, nil
}
func (f *fakeRepository) RevokeSession(_ context.Context, hash string) error {
	f.revokedHash = hash
	return nil
}

func newTestService(t *testing.T, repo Repository) *Service {
	t.Helper()
	s, err := NewService(repo, strings.Repeat("k", 32), "issuer", "audience", 15*time.Minute, 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	s.now = func() time.Time { return time.Unix(1_700_000_000, 0) }
	return s
}

func TestPasswordHashAndVerify(t *testing.T) {
	hash, err := HashPassword("correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "correct-horse-battery" || !VerifyPassword(hash, "correct-horse-battery") {
		t.Fatal("valid password was not safely hashed and verified")
	}
	if VerifyPassword(hash, "wrong-password-value") {
		t.Fatal("wrong password verified")
	}
}

func TestVerifyPasswordRejectsUnsafeParameters(t *testing.T) {
	for _, hash := range []string{
		"$argon2id$v=19$m=0,t=1,p=4$c2FsdA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=16$m=65536,t=1,p=4$c2FsdA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	} {
		if VerifyPassword(hash, "correct-horse-battery") {
			t.Fatalf("accepted malformed hash")
		}
	}
}

func TestNormalizeEmail(t *testing.T) {
	got, err := NormalizeEmail("  PERSON@Example.COM ")
	if err != nil || got != "person@example.com" {
		t.Fatalf("NormalizeEmail = %q, %v", got, err)
	}
	for _, email := range []string{"missing-at.example", "@example.com", "person@", "person@@example.com", "person @example.com"} {
		if _, err := NormalizeEmail(email); err == nil {
			t.Fatalf("accepted invalid email %q", email)
		}
	}
}

func TestRefreshHashIsDeterministicAndOpaque(t *testing.T) {
	one := RefreshHash("raw-refresh-token")
	if one != RefreshHash("raw-refresh-token") || one == "raw-refresh-token" {
		t.Fatal("refresh hashing failed")
	}
}

func TestAccessTokenRejectsTamperingAndExpiry(t *testing.T) {
	repo := &fakeRepository{user: User{ID: "user-1", Email: "person@example.com", Status: "active"}}
	s := newTestService(t, repo)
	result, err := s.issue(repo.user)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := s.ParseAccess(result.AccessToken); err != nil || id != "user-1" {
		t.Fatalf("valid token rejected: %q %v", id, err)
	}
	if _, err := s.ParseAccess(result.AccessToken + "x"); err == nil {
		t.Fatal("tampered token accepted")
	}
	s.now = func() time.Time { return time.Unix(1_700_001_000, 0) }
	if _, err := s.ParseAccess(result.AccessToken); err == nil {
		t.Fatal("expired token accepted")
	}
}

func TestRegisterLoginRefreshLogoutFlow(t *testing.T) {
	repo := &fakeRepository{}
	s := newTestService(t, repo)
	registered, err := s.Register(context.Background(), " PERSON@example.com ", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	if registered.User.Email != "person@example.com" || repo.passwordHash == "correct-horse-battery" || repo.refreshHash == RefreshHash("") {
		t.Fatal("registration did not persist safe normalized values")
	}
	loggedIn, err := s.Login(context.Background(), "person@example.com", "correct-horse-battery")
	if err != nil {
		t.Fatal(err)
	}
	refreshed, err := s.Refresh(context.Background(), loggedIn.RefreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if refreshed.RefreshToken == loggedIn.RefreshToken || repo.rotatedHash != RefreshHash(loggedIn.RefreshToken) {
		t.Fatal("refresh token was not rotated")
	}
	if _, err := s.Refresh(context.Background(), loggedIn.RefreshToken); err == nil {
		t.Fatal("replayed refresh token accepted")
	}
	if err := s.Logout(context.Background(), refreshed.RefreshToken); err != nil || repo.revokedHash != RefreshHash(refreshed.RefreshToken) {
		t.Fatal("logout did not revoke refresh token")
	}
}
