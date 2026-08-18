package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/domain/download"
	"github.com/face-search-ai/api/internal/http/middleware"
	"github.com/face-search-ai/api/internal/observability"
	"github.com/face-search-ai/api/internal/ratelimit"
)

// Downloads serves the public, customer-facing controlled-download endpoint.
// Authorization derives only from the opaque public Event token and the Event
// download policy; organization membership is never consulted here.
type Downloads struct {
	service       *download.Service
	limiter       *ratelimit.Limiter
	authorization *authorization.Service
}

func NewDownloads(service *download.Service, limiter *ratelimit.Limiter, authorizationService *authorization.Service) *Downloads {
	return &Downloads{service: service, limiter: limiter, authorization: authorizationService}
}

type publicDownloadRequest struct {
	PhotoIDs []string `json:"photoIds"`
}

type publicDownloadItem struct {
	PhotoID   string    `json:"photoId"`
	URL       string    `json:"url"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type publicDownloadResponse struct {
	Downloads []publicDownloadItem `json:"downloads"`
}

func (h *Downloads) Public(w http.ResponseWriter, r *http.Request) {
	token := r.PathValue("publicToken")

	if h.limiter != nil && !h.limiter.Allow(token+"|"+middleware.ClientIP(r)) {
		observability.RecordRateLimitRejection("download")
		h.audit(r, "", "", "download.rate_limited", authorization.AuditDenied, map[string]string{"reason": "rate_limited"})
		writeAuthError(w, http.StatusTooManyRequests, "rate_limited", "Too many download requests. Please try again shortly.")
		return
	}

	var request publicDownloadRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}

	result, err := h.service.Issue(r.Context(), token, request.PhotoIDs)
	h.auditResult(r, result)
	if err != nil {
		switch {
		case errors.Is(err, download.ErrInvalidRequest):
			writeAuthError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
		case errors.Is(err, download.ErrUnavailable):
			writeAuthError(w, http.StatusServiceUnavailable, "service_unavailable", "Downloads are temporarily unavailable.")
		default:
			writeAuthError(w, http.StatusNotFound, "not_found", "Resource not found.")
		}
		return
	}

	items := make([]publicDownloadItem, 0, len(result.Grants))
	for _, grant := range result.Grants {
		items = append(items, publicDownloadItem{PhotoID: grant.PhotoID, URL: grant.URL, ExpiresAt: grant.ExpiresAt})
	}
	writeAuthJSON(w, http.StatusOK, publicDownloadResponse{Downloads: items})
}

// auditResult emits a request-correlated audit record for a resolved-scope
// outcome. It is skipped when no trusted scope was resolved (unknown Event), so
// the audit trail never records enumeration attempts against a tenant.
func (h *Downloads) auditResult(r *http.Request, result download.Result) {
	if result.OrganizationID == "" {
		return
	}
	metadata := map[string]string{"kind": string(result.Kind)}
	action := "download.issue"
	outcome := authorization.AuditSuccess
	if result.Decision == download.DecisionDenied {
		action = "download.denied"
		outcome = authorization.AuditDenied
		if result.DenialCode != "" {
			metadata["denial"] = result.DenialCode
		}
	} else {
		metadata["count"] = strconv.Itoa(len(result.Grants))
	}
	// Decision and kind are bounded, low-cardinality classes; no token, URL, object
	// path, or photo identifier is recorded as a metric label.
	observability.RecordDownloadDecision(string(result.Decision), string(result.Kind))
	h.audit(r, result.OrganizationID, result.EventID, action, outcome, metadata)
}

func (h *Downloads) audit(r *http.Request, organizationID, eventID, action string, outcome authorization.AuditOutcome, metadata map[string]string) {
	if h.authorization == nil {
		return
	}
	h.authorization.Audit(r.Context(), authorization.AuditRecord{
		OrganizationID: organizationID,
		Action:         action,
		ResourceType:   "download",
		ResourceID:     eventID,
		Outcome:        outcome,
		RequestID:      middleware.RequestIDFromContext(r.Context()),
		Metadata:       metadata,
	})
}
