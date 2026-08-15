package photo

import (
	"context"
	"strings"
	"time"
)

const minimumMultipartPartSize int64 = 5 * 1024 * 1024

type UploadPolicy struct {
	MaxByteSize int64
	PartSize    int64
	MaxParts    int
	SignTTL     time.Duration
	SessionTTL  time.Duration
}

func NewUploadPolicy(maxByteSize, partSize int64, maxParts int, signTTL, sessionTTL time.Duration) (UploadPolicy, error) {
	if maxByteSize <= 0 || partSize < minimumMultipartPartSize || maxParts < 1 || maxParts > 10000 || signTTL <= 0 || signTTL > 24*time.Hour || sessionTTL <= signTTL || sessionTTL > 7*24*time.Hour {
		return UploadPolicy{}, ErrInvalid
	}
	if (maxByteSize+partSize-1)/partSize > int64(maxParts) {
		return UploadPolicy{}, ErrInvalid
	}
	return UploadPolicy{MaxByteSize: maxByteSize, PartSize: partSize, MaxParts: maxParts, SignTTL: signTTL, SessionTTL: sessionTTL}, nil
}

func (p UploadPolicy) PartCount(byteSize int64) int {
	if byteSize <= 0 {
		return 0
	}
	return int((byteSize + p.PartSize - 1) / p.PartSize)
}

func (p UploadPolicy) ValidatePart(partNumber int) error {
	if partNumber < 1 || partNumber > p.MaxParts {
		return ErrInvalid
	}
	return nil
}

type UploadSessionRepository interface {
	FindActive(context.Context, string, string, string, time.Time) (UploadSession, bool, error)
	Create(context.Context, string, string, string, string, time.Time) (UploadSession, error)
	MarkCompleted(context.Context, string, string, string, string) error
	MarkAborted(context.Context, string, string, string, string) error
}

type MultipartStorage interface {
	Initiate(context.Context, string, string, string) (string, error)
	SignPart(context.Context, string, string, int, time.Duration) (string, error)
	Complete(context.Context, string, string, []CompletedPart) error
	Abort(context.Context, string, string) error
	Stat(context.Context, string) (StoredObject, error)
}

type StoredObject struct {
	ByteSize       int64
	ContentType    string
	ChecksumSHA256 string
}

type UploadService struct {
	photos   Repository
	sessions UploadSessionRepository
	storage  MultipartStorage
	policy   UploadPolicy
	now      func() time.Time
}

func NewUploadService(photos Repository, sessions UploadSessionRepository, storage MultipartStorage, policy UploadPolicy) *UploadService {
	return &UploadService{photos: photos, sessions: sessions, storage: storage, policy: policy, now: time.Now}
}

func (s *UploadService) Initiate(ctx context.Context, organizationID, eventID, photoID string) (InitiateUploadView, error) {
	item, err := s.photos.Find(ctx, organizationID, eventID, photoID)
	if err != nil || item.Status != StatusPending && item.Status != StatusUploading || item.ByteSize > s.policy.MaxByteSize {
		return InitiateUploadView{}, ErrInvalid
	}
	now := s.now()
	session, found, err := s.sessions.FindActive(ctx, organizationID, eventID, photoID, now)
	if err != nil {
		return InitiateUploadView{}, err
	}
	if !found {
		uploadID, storageErr := s.storage.Initiate(ctx, item.ObjectKey, item.ContentType, item.ChecksumSHA256)
		if storageErr != nil {
			return InitiateUploadView{}, storageErr
		}
		session, err = s.sessions.Create(ctx, organizationID, eventID, photoID, uploadID, now.Add(s.policy.SessionTTL))
		if err != nil {
			_ = s.storage.Abort(ctx, item.ObjectKey, uploadID)
			return InitiateUploadView{}, err
		}
	}
	return InitiateUploadView{PhotoID: photoID, UploadID: session.UploadID, PartSize: s.policy.PartSize, PartCount: s.policy.PartCount(item.ByteSize), ExpiresAt: session.ExpiresAt}, nil
}

func (s *UploadService) SignPart(ctx context.Context, organizationID, eventID, photoID, uploadID string, partNumber int) (SignedPartView, error) {
	if err := s.policy.ValidatePart(partNumber); err != nil {
		return SignedPartView{}, err
	}
	session, found, err := s.sessions.FindActive(ctx, organizationID, eventID, photoID, s.now())
	if err != nil {
		return SignedPartView{}, err
	}
	if !found || session.UploadID != uploadID {
		return SignedPartView{}, ErrInvalid
	}
	item, err := s.photos.Find(ctx, organizationID, eventID, photoID)
	if err != nil || partNumber > s.policy.PartCount(item.ByteSize) {
		return SignedPartView{}, ErrInvalid
	}
	url, err := s.storage.SignPart(ctx, item.ObjectKey, uploadID, partNumber, s.policy.SignTTL)
	if err != nil {
		return SignedPartView{}, err
	}
	return SignedPartView{PartNumber: partNumber, URL: url, ExpiresAt: s.now().Add(s.policy.SignTTL)}, nil
}

func (s *UploadService) Complete(ctx context.Context, organizationID, eventID, photoID, uploadID string, parts []CompletedPart) (View, error) {
	parts, err := NewCompletedParts(parts, s.policy.MaxParts)
	if err != nil {
		return View{}, err
	}
	session, found, err := s.sessions.FindActive(ctx, organizationID, eventID, photoID, s.now())
	if err != nil {
		return View{}, err
	}
	if !found || session.UploadID != uploadID {
		return View{}, ErrInvalid
	}
	item, err := s.photos.Find(ctx, organizationID, eventID, photoID)
	if err != nil || len(parts) != s.policy.PartCount(item.ByteSize) {
		return View{}, ErrInvalid
	}
	if err := s.storage.Complete(ctx, item.ObjectKey, uploadID, parts); err != nil {
		return View{}, err
	}
	stored, err := s.storage.Stat(ctx, item.ObjectKey)
	if err != nil || stored.ByteSize != item.ByteSize || stored.ContentType != item.ContentType || item.ChecksumSHA256 != "" && stored.ChecksumSHA256 != item.ChecksumSHA256 {
		return View{}, ErrInvalid
	}
	if err := s.sessions.MarkCompleted(ctx, organizationID, eventID, photoID, uploadID); err != nil {
		return View{}, err
	}
	item.Status = StatusUploaded
	return item.View(), nil
}

func (s *UploadService) Abort(ctx context.Context, organizationID, eventID, photoID, uploadID string) error {
	session, found, err := s.sessions.FindActive(ctx, organizationID, eventID, photoID, s.now())
	if err != nil {
		return err
	}
	if !found {
		return nil
	}
	if session.UploadID != uploadID {
		return ErrInvalid
	}
	item, err := s.photos.Find(ctx, organizationID, eventID, photoID)
	if err != nil {
		return err
	}
	if err := s.storage.Abort(ctx, item.ObjectKey, uploadID); err != nil {
		return err
	}
	return s.sessions.MarkAborted(ctx, organizationID, eventID, photoID, uploadID)
}

type UploadSessionStatus string

const (
	UploadSessionActive    UploadSessionStatus = "active"
	UploadSessionCompleted UploadSessionStatus = "completed"
	UploadSessionAborted   UploadSessionStatus = "aborted"
	UploadSessionExpired   UploadSessionStatus = "expired"
)

type UploadSession struct {
	ID             string
	OrganizationID string
	EventID        string
	PhotoID        string
	UploadID       string
	ObjectKey      string
	ContentType    string
	ByteSize       int64
	ChecksumSHA256 string
	Status         UploadSessionStatus
	ExpiresAt      time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func (s UploadSession) ActiveAt(now time.Time) bool {
	return s.Status == UploadSessionActive && now.Before(s.ExpiresAt)
}

type InitiateUploadView struct {
	PhotoID   string    `json:"photoId"`
	UploadID  string    `json:"uploadId"`
	PartSize  int64     `json:"partSize"`
	PartCount int       `json:"partCount"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type SignedPartView struct {
	PartNumber int       `json:"partNumber"`
	URL        string    `json:"url"`
	ExpiresAt  time.Time `json:"expiresAt"`
}

type CompletedPart struct {
	PartNumber int    `json:"partNumber"`
	ETag       string `json:"etag"`
}

func NewCompletedParts(parts []CompletedPart, maxParts int) ([]CompletedPart, error) {
	if len(parts) == 0 || len(parts) > maxParts {
		return nil, ErrInvalid
	}
	validated := make([]CompletedPart, len(parts))
	previous := 0
	for index, part := range parts {
		part.ETag = strings.Trim(strings.TrimSpace(part.ETag), `"`)
		if part.PartNumber <= previous || part.PartNumber < 1 || part.PartNumber > maxParts || part.ETag == "" || len(part.ETag) > 200 {
			return nil, ErrInvalid
		}
		validated[index] = part
		previous = part.PartNumber
	}
	return validated, nil
}
