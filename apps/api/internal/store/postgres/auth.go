package postgres

import (
	"context"
	"time"

	"github.com/face-search-ai/api/internal/domain/auth"
	"github.com/face-search-ai/api/internal/store"
	"github.com/google/uuid"
)

type AuthRepository struct{ db *Store }

func NewAuthRepository(db *Store) *AuthRepository { return &AuthRepository{db: db} }

func scanUser(row interface{ Scan(...any) error }) (auth.User, error) {
	var u auth.User
	err := row.Scan(&u.ID, &u.Email, &u.Status, &u.CreatedAt)
	if err != nil {
		return auth.User{}, MapError(err)
	}
	return u, nil
}
func (r *AuthRepository) CreateUserWithSession(ctx context.Context, email, passwordHash, refreshHash string, expires time.Time) (auth.User, error) {
	var user auth.User
	err := r.db.WithinTransaction(ctx, func(ctx context.Context, tx store.DBTX) error {
		var err error
		user, err = scanUser(tx.QueryRow(ctx, "INSERT INTO users (email,password_hash) VALUES ($1,$2) RETURNING id,email,status,created_at", email, passwordHash))
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, "INSERT INTO auth_sessions (user_id,refresh_token_hash,family_id,expires_at) VALUES ($1,$2,$3,$4)", user.ID, refreshHash, uuid.New(), expires)
		return MapError(err)
	})
	return user, err
}
func (r *AuthRepository) FindUserByEmail(ctx context.Context, email string) (auth.User, string, error) {
	var u auth.User
	var hash string
	err := r.db.QueryRow(ctx, "SELECT id,email,status,created_at,password_hash FROM users WHERE email=$1", email).Scan(&u.ID, &u.Email, &u.Status, &u.CreatedAt, &hash)
	return u, hash, MapError(err)
}
func (r *AuthRepository) FindUserByID(ctx context.Context, id string) (auth.User, error) {
	return scanUser(r.db.QueryRow(ctx, "SELECT id,email,status,created_at FROM users WHERE id=$1", id))
}
func (r *AuthRepository) CreateSession(ctx context.Context, userID, hash string, expires time.Time) (auth.Session, error) {
	family := uuid.New()
	var s auth.Session
	err := r.db.QueryRow(ctx, "INSERT INTO auth_sessions (user_id,refresh_token_hash,family_id,expires_at) VALUES ($1,$2,$3,$4) RETURNING id,user_id,family_id,status,expires_at", userID, hash, family, expires).Scan(&s.ID, &s.UserID, &s.FamilyID, &s.Status, &s.ExpiresAt)
	return s, MapError(err)
}
func (r *AuthRepository) RotateSession(ctx context.Context, oldHash, newHash string, expires time.Time) (auth.Session, error) {
	var replacement auth.Session
	err := r.db.WithinTransaction(ctx, func(ctx context.Context, tx store.DBTX) error {
		var old auth.Session
		err := tx.QueryRow(ctx, "SELECT id,user_id,family_id,status,expires_at FROM auth_sessions WHERE refresh_token_hash=$1 FOR UPDATE", oldHash).Scan(&old.ID, &old.UserID, &old.FamilyID, &old.Status, &old.ExpiresAt)
		if err != nil {
			return MapError(err)
		}
		if old.Status != "active" || !old.ExpiresAt.After(time.Now()) {
			return auth.ErrInvalidCredentials
		}
		err = tx.QueryRow(ctx, "INSERT INTO auth_sessions (user_id,refresh_token_hash,family_id,expires_at) VALUES ($1,$2,$3,$4) RETURNING id,user_id,family_id,status,expires_at", old.UserID, newHash, old.FamilyID, expires).Scan(&replacement.ID, &replacement.UserID, &replacement.FamilyID, &replacement.Status, &replacement.ExpiresAt)
		if err != nil {
			return MapError(err)
		}
		tag, err := tx.Exec(ctx, "UPDATE auth_sessions SET status='rotated',replaced_by_session_id=$1,last_used_at=now() WHERE id=$2 AND status='active'", replacement.ID, old.ID)
		if err != nil {
			return MapError(err)
		}
		if tag.RowsAffected() != 1 {
			return auth.ErrInvalidCredentials
		}
		return nil
	})
	return replacement, err
}
func (r *AuthRepository) RevokeSession(ctx context.Context, hash string) error {
	_, err := r.db.Exec(ctx, "UPDATE auth_sessions SET status='revoked',revoked_at=now() WHERE refresh_token_hash=$1 AND status='active'", hash)
	if err != nil {
		return MapError(err)
	}
	return nil
}

var _ auth.Repository = (*AuthRepository)(nil)
