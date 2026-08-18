package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/domain/event"
	"github.com/face-search-ai/api/internal/http/middleware"
)

// maxJSONBodyBytes bounds JSON request bodies for the auth, event, photo, and
// download endpoints. It is a deliberately tight cap relative to the multipart
// selfie-search limit: these endpoints only carry small structured payloads (the
// largest being a completed-upload part list). Oversized bodies are rejected with
// a safe 413 before any decoding work.
const maxJSONBodyBytes = 1 << 20 // 1 MiB

type Events struct {
	events        *event.Service
	authorization *authorization.Service
}

func NewEvents(events *event.Service, authorizationService *authorization.Service) *Events {
	return &Events{events: events, authorization: authorizationService}
}

type createEventRequest struct {
	Name             string           `json:"name"`
	Visibility       event.Visibility `json:"visibility"`
	ExpiresAt        *time.Time       `json:"expiresAt"`
	DownloadsEnabled bool             `json:"downloadsEnabled"`
	MatchThreshold   *float64         `json:"matchThreshold"`
}

type updateEventRequest struct {
	Name             *string           `json:"name"`
	Visibility       *event.Visibility `json:"visibility"`
	ExpiresAt        **time.Time       `json:"expiresAt"`
	DownloadsEnabled *bool             `json:"downloadsEnabled"`
	MatchThreshold   **float64         `json:"matchThreshold"`
}

func (h *Events) Public(w http.ResponseWriter, r *http.Request) {
	result, err := h.events.FindPublic(r.Context(), r.PathValue("publicToken"), time.Now().UTC())
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	writeAuthJSON(w, http.StatusOK, result)
}

func (h *Events) Create(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionEventWrite, "event.create")
	if !ok {
		return
	}
	var request createEventRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	command, err := event.NewCreateCommand(request.Name, request.Visibility, request.ExpiresAt, request.DownloadsEnabled, request.MatchThreshold)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
		return
	}
	created, err := h.events.Create(r.Context(), tenant.OrganizationID, tenant.UserID, command)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "request_failed", "Request could not be completed.")
		return
	}
	h.audit(r, tenant, "event.create", created.ID, authorization.AuditSuccess)
	writeAuthJSON(w, http.StatusCreated, created)
}

func (h *Events) List(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionEventRead, "event.list")
	if !ok {
		return
	}
	results, err := h.events.List(r.Context(), tenant.OrganizationID)
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	writeAuthJSON(w, http.StatusOK, results)
}

func (h *Events) Get(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionEventRead, "event.read")
	if !ok {
		return
	}
	result, err := h.events.Find(r.Context(), tenant.OrganizationID, r.PathValue("eventId"))
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	h.audit(r, tenant, "event.read", result.ID, authorization.AuditSuccess)
	writeAuthJSON(w, http.StatusOK, result)
}

func (h *Events) Update(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionEventWrite, "event.update")
	if !ok {
		return
	}
	var request updateEventRequest
	if err := decodeStrictJSON(w, r, &request); err != nil {
		writeDecodeError(w, err)
		return
	}
	command, err := event.NewUpdateCommand(request.Name, request.Visibility, request.ExpiresAt, request.DownloadsEnabled, request.MatchThreshold)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
		return
	}
	result, err := h.events.Update(r.Context(), tenant.OrganizationID, r.PathValue("eventId"), command)
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	h.audit(r, tenant, "event.update", result.ID, authorization.AuditSuccess)
	writeAuthJSON(w, http.StatusOK, result)
}

func (h *Events) Archive(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionEventWrite, "event.archive")
	if !ok {
		return
	}
	eventID := r.PathValue("eventId")
	if err := h.events.Archive(r.Context(), tenant.OrganizationID, eventID); err != nil {
		h.writeUnavailable(w)
		return
	}
	h.audit(r, tenant, "event.archive", eventID, authorization.AuditSuccess)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Events) Status(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionEventRead, "event.status.read")
	if !ok {
		return
	}
	status, err := h.events.Status(r.Context(), tenant.OrganizationID, r.PathValue("eventId"))
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	writeAuthJSON(w, http.StatusOK, status)
}

func (h *Events) authorize(w http.ResponseWriter, r *http.Request, permission authorization.Permission, action string) (authorization.TenantPrincipal, bool) {
	principal, ok := authorization.PrincipalFromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return authorization.TenantPrincipal{}, false
	}
	tenant, err := h.authorization.Authorize(r.Context(), principal.UserID, r.PathValue("organizationId"), permission)
	if err != nil {
		h.authorization.Audit(r.Context(), authorization.AuditRecord{ActorUserID: principal.UserID, Action: action, ResourceType: "event", Outcome: authorization.AuditDenied, RequestID: middleware.RequestIDFromContext(r.Context())})
		h.writeUnavailable(w)
		return authorization.TenantPrincipal{}, false
	}
	return tenant, true
}

func (h *Events) audit(r *http.Request, tenant authorization.TenantPrincipal, action, resourceID string, outcome authorization.AuditOutcome) {
	h.authorization.Audit(r.Context(), authorization.AuditRecord{OrganizationID: tenant.OrganizationID, ActorUserID: tenant.UserID, Action: action, ResourceType: "event", ResourceID: resourceID, Outcome: outcome, RequestID: middleware.RequestIDFromContext(r.Context())})
}

func (h *Events) writeUnavailable(w http.ResponseWriter) {
	writeAuthError(w, http.StatusNotFound, "not_found", "Resource not found.")
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, target any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxJSONBodyBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return io.ErrUnexpectedEOF
	}
	return nil
}

// writeDecodeError maps a request-body decode failure to a safe response: 413 when
// the body exceeded the endpoint cap and 400 otherwise. Both use the shared error
// shape and never expose parser internals.
func writeDecodeError(w http.ResponseWriter, err error) {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		writeAuthError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body is too large.")
		return
	}
	writeAuthError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
}
