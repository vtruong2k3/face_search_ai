package handlers

import (
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/face-search-ai/api/internal/domain/search"
	"github.com/face-search-ai/api/internal/observability"
)

const maxSearchRequestBytes = search.MaxSelfieBytes + 64*1024

type Search struct{ service *search.Service }

func NewSearch(service *search.Service) *Search { return &Search{service: service} }

type publicSearchResponse struct {
	Results    []search.Result `json:"results"`
	NextCursor *string         `json:"nextCursor"`
}

func (h *Search) Public(w http.ResponseWriter, r *http.Request) {
	// outcome is a bounded class recorded with the request latency. It never
	// contains the token, selfie, or any embedding.
	started := time.Now()
	outcome := "ok"
	defer func() { observability.RecordSearch(outcome, time.Since(started)) }()

	r.Body = http.MaxBytesReader(w, r.Body, maxSearchRequestBytes)
	if err := r.ParseMultipartForm(maxSearchRequestBytes); err != nil {
		var maxBytes *http.MaxBytesError
		if errors.As(err, &maxBytes) {
			outcome = string(search.CodeSelfieTooLarge)
			writeSearchError(w, http.StatusRequestEntityTooLarge, string(search.CodeSelfieTooLarge), "The selfie is too large.")
			return
		}
		outcome = "invalid_request"
		writeSearchError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
		return
	}
	consent := r.FormValue("consent")
	consentVersion := r.FormValue("consentVersion")
	file, header, err := r.FormFile("selfie")
	if err != nil {
		outcome = string(search.CodeInvalidImage)
		writeSearchError(w, http.StatusUnprocessableEntity, string(search.CodeInvalidImage), "A valid selfie is required.")
		return
	}
	defer file.Close()
	if header.Size > search.MaxSelfieBytes {
		outcome = string(search.CodeSelfieTooLarge)
		writeSearchError(w, http.StatusRequestEntityTooLarge, string(search.CodeSelfieTooLarge), "The selfie is too large.")
		return
	}
	contentType := header.Header.Get("Content-Type")
	if contentType == "" {
		contentType = r.Header.Get("Content-Type")
	}
	selfie, err := io.ReadAll(io.LimitReader(file, search.MaxSelfieBytes+1))
	if err != nil {
		outcome = "invalid_request"
		writeSearchError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
		return
	}
	if int64(len(selfie)) > search.MaxSelfieBytes {
		for i := range selfie {
			selfie[i] = 0
		}
		outcome = string(search.CodeSelfieTooLarge)
		writeSearchError(w, http.StatusRequestEntityTooLarge, string(search.CodeSelfieTooLarge), "The selfie is too large.")
		return
	}
	results, err := h.service.Search(r.Context(), r.PathValue("publicToken"), contentType, selfie, consent, consentVersion)
	if err != nil {
		outcome = searchOutcome(err)
		writePublicSearchFailure(w, err)
		return
	}
	writeAuthJSON(w, http.StatusOK, publicSearchResponse{Results: results, NextCursor: nil})
}

// searchOutcome maps a search failure to a bounded outcome class for metrics. It
// mirrors the HTTP mapping in writePublicSearchFailure and never returns
// unbounded detail.
func searchOutcome(err error) string {
	var policy search.PolicyError
	var faces search.FaceCountError
	switch {
	case errors.As(err, &policy):
		return string(policy.Code)
	case errors.As(err, &faces):
		return string(faces.Code())
	case errors.Is(err, search.ErrUnavailable):
		return "service_unavailable"
	default:
		return "not_found"
	}
}

func writePublicSearchFailure(w http.ResponseWriter, err error) {
	var policy search.PolicyError
	var faces search.FaceCountError
	switch {
	case errors.As(err, &policy):
		writeSearchError(w, http.StatusUnprocessableEntity, string(policy.Code), publicSearchMessage(policy.Code))
	case errors.As(err, &faces):
		writeSearchError(w, http.StatusUnprocessableEntity, string(faces.Code()), publicSearchMessage(faces.Code()))
	case errors.Is(err, search.ErrUnavailable):
		writeSearchError(w, http.StatusServiceUnavailable, "service_unavailable", "Search is temporarily unavailable.")
	default:
		writeSearchError(w, http.StatusNotFound, "not_found", "Resource not found.")
	}
}

func publicSearchMessage(code search.ErrorCode) string {
	switch code {
	case search.CodeConsentRequired:
		return "Consent is required."
	case search.CodeUnsupportedMedia:
		return "The selfie format is not supported."
	case search.CodeSelfieTooLarge:
		return "The selfie is too large."
	case search.CodeFaceCountZero:
		return "Exactly one face is required."
	case search.CodeFaceCountMultiple:
		return "Exactly one face is required."
	default:
		return "The selfie is invalid."
	}
}

func writeSearchError(w http.ResponseWriter, status int, code, message string) {
	writeAuthError(w, status, code, message)
}
