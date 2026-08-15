package handlers

import (
	"net/http"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/domain/photo"
	"github.com/face-search-ai/api/internal/http/middleware"
)

type Photos struct {
	photos        *photo.Service
	authorization *authorization.Service
}

func NewPhotos(photos *photo.Service, authorizationService *authorization.Service) *Photos {
	return &Photos{photos: photos, authorization: authorizationService}
}

type createPhotoRequest struct {
	OriginalFilename string `json:"originalFilename"`
	ContentType      string `json:"contentType"`
	ByteSize         int64  `json:"byteSize"`
	ChecksumSHA256   string `json:"checksumSha256"`
}

func (h *Photos) Create(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionPhotoWrite, "photo.create")
	if !ok {
		return
	}
	var request createPhotoRequest
	if err := decodeStrictJSON(r, &request); err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
		return
	}
	command, err := photo.NewCreateCommand(request.OriginalFilename, request.ContentType, request.ByteSize, request.ChecksumSHA256)
	if err != nil {
		writeAuthError(w, http.StatusBadRequest, "invalid_request", "Request is invalid.")
		return
	}
	created, err := h.photos.Create(r.Context(), tenant.OrganizationID, r.PathValue("eventId"), tenant.UserID, command)
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	h.audit(r, tenant, "photo.create", created.ID, authorization.AuditSuccess)
	writeAuthJSON(w, http.StatusCreated, created)
}

func (h *Photos) List(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionPhotoRead, "photo.list")
	if !ok {
		return
	}
	results, err := h.photos.List(r.Context(), tenant.OrganizationID, r.PathValue("eventId"))
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	writeAuthJSON(w, http.StatusOK, results)
}

func (h *Photos) Get(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionPhotoRead, "photo.read")
	if !ok {
		return
	}
	result, err := h.photos.Find(r.Context(), tenant.OrganizationID, r.PathValue("eventId"), r.PathValue("photoId"))
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	writeAuthJSON(w, http.StatusOK, result)
}

func (h *Photos) Delete(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionPhotoWrite, "photo.delete")
	if !ok {
		return
	}
	photoID := r.PathValue("photoId")
	if err := h.photos.Delete(r.Context(), tenant.OrganizationID, r.PathValue("eventId"), photoID); err != nil {
		h.writeUnavailable(w)
		return
	}
	h.audit(r, tenant, "photo.delete", photoID, authorization.AuditSuccess)
	w.WriteHeader(http.StatusNoContent)
}

func (h *Photos) Reprocess(w http.ResponseWriter, r *http.Request) {
	tenant, ok := h.authorize(w, r, authorization.PermissionPhotoWrite, "photo.reprocess")
	if !ok {
		return
	}
	result, err := h.photos.Reprocess(r.Context(), tenant.OrganizationID, r.PathValue("eventId"), r.PathValue("photoId"))
	if err != nil {
		h.writeUnavailable(w)
		return
	}
	h.audit(r, tenant, "photo.reprocess", result.ID, authorization.AuditSuccess)
	writeAuthJSON(w, http.StatusOK, result)
}

func (h *Photos) authorize(w http.ResponseWriter, r *http.Request, permission authorization.Permission, action string) (authorization.TenantPrincipal, bool) {
	principal, ok := authorization.PrincipalFromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return authorization.TenantPrincipal{}, false
	}
	tenant, err := h.authorization.Authorize(r.Context(), principal.UserID, r.PathValue("organizationId"), permission)
	if err != nil {
		h.authorization.Audit(r.Context(), authorization.AuditRecord{ActorUserID: principal.UserID, Action: action, ResourceType: "photo", Outcome: authorization.AuditDenied, RequestID: middleware.RequestIDFromContext(r.Context())})
		h.writeUnavailable(w)
		return authorization.TenantPrincipal{}, false
	}
	return tenant, true
}

func (h *Photos) audit(r *http.Request, tenant authorization.TenantPrincipal, action, resourceID string, outcome authorization.AuditOutcome) {
	h.authorization.Audit(r.Context(), authorization.AuditRecord{OrganizationID: tenant.OrganizationID, ActorUserID: tenant.UserID, Action: action, ResourceType: "photo", ResourceID: resourceID, Outcome: outcome, RequestID: middleware.RequestIDFromContext(r.Context())})
}

func (h *Photos) writeUnavailable(w http.ResponseWriter) {
	writeAuthError(w, http.StatusNotFound, "not_found", "Resource not found.")
}
