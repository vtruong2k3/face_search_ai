package search

import (
	"errors"
	"testing"
)

func validRequest() Request {
	return Request{
		ContentType: "image/jpeg", SelfieBytes: 1024,
		Consent: "true", ConsentVersion: "2026-01",
		Limit: 20,
	}
}

func TestValidateRequestAcceptsBoundedConsentedImage(t *testing.T) {
	if err := ValidateRequest(validRequest()); err != nil {
		t.Fatalf("ValidateRequest() error = %v", err)
	}
}

func TestValidateRequestRejectsMissingConsent(t *testing.T) {
	request := validRequest()
	request.Consent = "false"
	var policy PolicyError
	if !errors.As(ValidateRequest(request), &policy) || policy.Code != CodeConsentRequired {
		t.Fatalf("expected consent policy error, got %v", ValidateRequest(request))
	}
}

func TestValidateRequestRejectsUnsupportedAndOversizedImages(t *testing.T) {
	request := validRequest()
	request.ContentType = "image/gif"
	err := ValidateRequest(request)
	if !errors.Is(err, PolicyError{Code: CodeUnsupportedMedia}) {
		t.Fatalf("unsupported media error = %v", err)
	}
	request = validRequest()
	request.SelfieBytes = MaxSelfieBytes + 1
	err = ValidateRequest(request)
	if !errors.Is(err, PolicyError{Code: CodeSelfieTooLarge}) {
		t.Fatalf("oversized image error = %v", err)
	}
}

func TestValidateRequestRejectsInvalidCursorAndLimit(t *testing.T) {
	request := validRequest()
	request.Cursor = string(make([]byte, MaxCursorBytes+1))
	err := ValidateRequest(request)
	if !errors.Is(err, PolicyError{Code: CodeInvalidCursor}) {
		t.Fatalf("invalid cursor error = %v", err)
	}
	request = validRequest()
	request.Limit = MaxResults + 1
	err = ValidateRequest(request)
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid limit error = %v", err)
	}
}

func TestRequireExactlyOneFace(t *testing.T) {
	if err := RequireExactlyOneFace(1); err != nil {
		t.Fatalf("one face error = %v", err)
	}
	if err := RequireExactlyOneFace(0); !errors.Is(err, FaceCountError{Count: 0}) {
		t.Fatalf("zero-face error = %v", err)
	}
	multiple := RequireExactlyOneFace(2)
	faceError, ok := multiple.(FaceCountError)
	if !ok || faceError.Code() != CodeFaceCountMultiple || faceError.Count != 2 {
		t.Fatalf("multiple-face error = %#v", multiple)
	}
}

func TestPublicResultOmitsSensitiveFields(t *testing.T) {
	result := (PublicResult{PhotoID: "photo-1"}).View()
	if result.PhotoID != "photo-1" {
		t.Fatalf("unexpected photo ID %q", result.PhotoID)
	}
}
