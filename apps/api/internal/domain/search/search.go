package search

import (
	"errors"
	"strings"
)

const (
	MaxSelfieBytes         = 10 * 1024 * 1024
	MaxCursorBytes         = 512
	MaxConsentVersionBytes = 64
	MaxResults             = 100
)

var ErrInvalidRequest = errors.New("invalid search request")

type ErrorCode string

const (
	CodeConsentRequired   ErrorCode = "consent_required"
	CodeInvalidImage      ErrorCode = "invalid_image"
	CodeUnsupportedMedia  ErrorCode = "unsupported_media_type"
	CodeSelfieTooLarge    ErrorCode = "selfie_too_large"
	CodeFaceCountZero     ErrorCode = "face_count_zero"
	CodeFaceCountMultiple ErrorCode = "face_count_multiple"
	CodeInvalidCursor     ErrorCode = "invalid_cursor"
)

type PolicyError struct {
	Code ErrorCode
}

func (e PolicyError) Error() string { return string(e.Code) }

func (e PolicyError) Is(target error) bool {
	other, ok := target.(PolicyError)
	return ok && e.Code == other.Code
}

type Request struct {
	ContentType    string
	SelfieBytes    int64
	Consent        string
	ConsentVersion string
	Cursor         string
	Limit          int
}

// ValidateRequest validates metadata only. It deliberately does not accept or retain
// selfie bytes; callers must stream and discard the image after bounded processing.
func ValidateRequest(request Request) error {
	if strings.ToLower(strings.TrimSpace(request.Consent)) != "true" {
		return PolicyError{Code: CodeConsentRequired}
	}
	version := strings.TrimSpace(request.ConsentVersion)
	if version == "" || len(version) > MaxConsentVersionBytes {
		return ErrInvalidRequest
	}
	if request.SelfieBytes <= 0 {
		return PolicyError{Code: CodeInvalidImage}
	}
	if request.SelfieBytes > MaxSelfieBytes {
		return PolicyError{Code: CodeSelfieTooLarge}
	}
	switch strings.ToLower(strings.TrimSpace(request.ContentType)) {
	case "image/jpeg", "image/png", "image/webp":
	default:
		return PolicyError{Code: CodeUnsupportedMedia}
	}
	if err := ValidateCursor(request.Cursor); err != nil {
		return err
	}
	if request.Limit < 0 || request.Limit > MaxResults {
		return ErrInvalidRequest
	}
	return nil
}

func ValidateCursor(cursor string) error {
	if len(cursor) > MaxCursorBytes {
		return PolicyError{Code: CodeInvalidCursor}
	}
	return nil
}

type FaceCountError struct{ Count int }

func (e FaceCountError) Error() string {
	if e.Count == 0 {
		return string(CodeFaceCountZero)
	}
	return string(CodeFaceCountMultiple)
}

func (e FaceCountError) Code() ErrorCode {
	if e.Count == 0 {
		return CodeFaceCountZero
	}
	return CodeFaceCountMultiple
}

func RequireExactlyOneFace(count int) error {
	if count == 1 {
		return nil
	}
	return FaceCountError{Count: count}
}

type Result struct {
	PhotoID string `json:"photoId"`
}

// PublicResult is the intentionally narrow response projection. It cannot carry
// storage keys, embeddings, face identifiers, scores, or uploader metadata.
type PublicResult struct{ PhotoID string }

func (r PublicResult) View() Result { return Result{PhotoID: r.PhotoID} }
