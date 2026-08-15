package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrEmailInUse         = errors.New("account cannot be created")
	ErrInvalidInput       = errors.New("invalid authentication input")
)

type User struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}
type Session struct {
	ID, UserID, FamilyID, Status string
	ExpiresAt                    time.Time
}
type Repository interface {
	CreateUserWithSession(context.Context, string, string, string, time.Time) (User, error)
	FindUserByEmail(context.Context, string) (User, string, error)
	FindUserByID(context.Context, string) (User, error)
	CreateSession(context.Context, string, string, time.Time) (Session, error)
	RotateSession(context.Context, string, string, time.Time) (Session, error)
	RevokeSession(context.Context, string) error
}

type Service struct {
	repo                  Repository
	signingKey            []byte
	issuer, audience      string
	accessTTL, refreshTTL time.Duration
	now                   func() time.Time
}
type Result struct {
	AccessToken, RefreshToken string
	ExpiresIn                 int64
	User                      User
}

func NewService(repo Repository, key, issuer, audience string, accessTTL, refreshTTL time.Duration) (*Service, error) {
	if len(key) < 32 || issuer == "" || audience == "" || accessTTL <= 0 || refreshTTL <= 0 {
		return nil, errors.New("invalid auth configuration")
	}
	return &Service{repo: repo, signingKey: []byte(key), issuer: issuer, audience: audience, accessTTL: accessTTL, refreshTTL: refreshTTL, now: time.Now}, nil
}
func NormalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" || len(email) > 254 || strings.Count(email, "@") != 1 || strings.ContainsAny(email, " \t\r\n") {
		return "", ErrInvalidInput
	}
	parts := strings.SplitN(email, "@", 2)
	if parts[0] == "" || parts[1] == "" || strings.HasPrefix(parts[1], ".") || strings.HasSuffix(parts[1], ".") || !strings.Contains(parts[1], ".") {
		return "", ErrInvalidInput
	}
	return email, nil
}
func validatePassword(p string) error {
	if len(p) < 12 || len(p) > 128 {
		return ErrInvalidInput
	}
	return nil
}

func HashPassword(password string) (string, error) {
	if err := validatePassword(password); err != nil {
		return "", err
	}
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	hash := argon2.IDKey([]byte(password), salt, 1, 64*1024, 4, 32)
	return fmt.Sprintf("$argon2id$v=19$m=65536,t=1,p=4$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(hash)), nil
}
func VerifyPassword(encoded, password string) bool {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false
	}
	if parts[2] != "v=19" {
		return false
	}
	var mem uint32
	var iter uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &mem, &iter, &threads); err != nil || mem < 8 || mem > 128*1024 || iter < 1 || iter > 4 || threads < 1 || threads > 8 {
		return false
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(salt) < 8 || len(salt) > 64 || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iter, mem, threads, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}
func newRefresh() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	sum := sha256.Sum256([]byte(token))
	return token, base64.RawURLEncoding.EncodeToString(sum[:]), nil
}
func RefreshHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (s *Service) issue(user User) (Result, error) {
	now := s.now()
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Result{}, err
	}
	claims := map[string]any{"sub": user.ID, "iss": s.issuer, "aud": s.audience, "iat": now.Unix(), "exp": now.Add(s.accessTTL).Unix(), "jti": base64.RawURLEncoding.EncodeToString(id)}
	header, _ := json.Marshal(map[string]string{"alg": "HS256", "typ": "JWT"})
	payload, _ := json.Marshal(claims)
	body := base64.RawURLEncoding.EncodeToString(header) + "." + base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(body))
	return Result{AccessToken: body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), ExpiresIn: int64(s.accessTTL.Seconds()), User: user}, nil
}
func (s *Service) ParseAccess(token string) (string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", ErrInvalidCredentials
	}
	headerData, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", ErrInvalidCredentials
	}
	var header struct {
		Algorithm string `json:"alg"`
		Type      string `json:"typ"`
	}
	if json.Unmarshal(headerData, &header) != nil || header.Algorithm != "HS256" || header.Type != "JWT" {
		return "", ErrInvalidCredentials
	}
	mac := hmac.New(sha256.New, s.signingKey)
	mac.Write([]byte(parts[0] + "." + parts[1]))
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return "", ErrInvalidCredentials
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", ErrInvalidCredentials
	}
	var claims struct {
		Subject  string `json:"sub"`
		Issuer   string `json:"iss"`
		Audience string `json:"aud"`
		IssuedAt int64  `json:"iat"`
		Expires  int64  `json:"exp"`
		TokenID  string `json:"jti"`
	}
	if json.Unmarshal(data, &claims) != nil || claims.Subject == "" || claims.Issuer != s.issuer || claims.Audience != s.audience || claims.IssuedAt <= 0 || claims.Expires <= claims.IssuedAt || claims.TokenID == "" || s.now().Unix() >= claims.Expires {
		return "", ErrInvalidCredentials
	}
	return claims.Subject, nil
}

func (s *Service) Register(ctx context.Context, email, password string) (Result, error) {
	email, err := NormalizeEmail(email)
	if err != nil || validatePassword(password) != nil {
		return Result{}, ErrInvalidInput
	}
	hash, err := HashPassword(password)
	if err != nil {
		return Result{}, err
	}
	var user User
	raw, refreshHash, err := newRefresh()
	if err != nil {
		return Result{}, err
	}
	user, err = s.repo.CreateUserWithSession(ctx, email, hash, refreshHash, s.now().Add(s.refreshTTL))
	if err != nil {
		return Result{}, err
	}
	r, err := s.issue(user)
	r.RefreshToken = raw
	return r, err
}
func (s *Service) Login(ctx context.Context, email, password string) (Result, error) {
	email, err := NormalizeEmail(email)
	if err != nil {
		return Result{}, ErrInvalidCredentials
	}
	user, hash, err := s.repo.FindUserByEmail(ctx, email)
	if err != nil || user.Status != "active" || !VerifyPassword(hash, password) {
		return Result{}, ErrInvalidCredentials
	}
	raw, rh, err := newRefresh()
	if err != nil {
		return Result{}, err
	}
	if _, err = s.repo.CreateSession(ctx, user.ID, rh, s.now().Add(s.refreshTTL)); err != nil {
		return Result{}, err
	}
	r, err := s.issue(user)
	r.RefreshToken = raw
	return r, err
}
func (s *Service) Refresh(ctx context.Context, token string) (Result, error) {
	if token == "" {
		return Result{}, ErrInvalidCredentials
	}
	raw, hash, err := newRefresh()
	if err != nil {
		return Result{}, err
	}
	session, err := s.repo.RotateSession(ctx, RefreshHash(token), hash, s.now().Add(s.refreshTTL))
	if err != nil {
		return Result{}, ErrInvalidCredentials
	}
	user, err := s.repo.FindUserByID(ctx, session.UserID)
	if err != nil || user.Status != "active" {
		return Result{}, ErrInvalidCredentials
	}
	r, err := s.issue(user)
	r.RefreshToken = raw
	return r, err
}
func (s *Service) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repo.RevokeSession(ctx, RefreshHash(token))
}
func (s *Service) Me(ctx context.Context, token string) (User, error) {
	id, err := s.ParseAccess(token)
	if err != nil {
		return User{}, err
	}
	return s.repo.FindUserByID(ctx, id)
}
