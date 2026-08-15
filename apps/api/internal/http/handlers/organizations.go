package handlers

import (
	"net/http"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/http/middleware"
)

type Organizations struct {
	service *authorization.Service
}

func NewOrganizations(service *authorization.Service) *Organizations {
	return &Organizations{service: service}
}

func (h *Organizations) List(w http.ResponseWriter, r *http.Request) {
	principal, ok := authorization.PrincipalFromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return
	}
	memberships, err := h.service.ListMemberships(r.Context(), principal.UserID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "request_failed", "Request could not be completed.")
		return
	}
	writeAuthJSON(w, http.StatusOK, memberships)
}

func (h *Organizations) Membership(w http.ResponseWriter, r *http.Request) {
	principal, ok := authorization.PrincipalFromContext(r.Context())
	if !ok {
		writeAuthError(w, http.StatusUnauthorized, "authentication_rejected", "Authentication request rejected.")
		return
	}
	organizationID := r.PathValue("organizationId")
	tenant, err := h.service.Authorize(r.Context(), principal.UserID, organizationID, authorization.PermissionOrganizationRead)
	if err != nil {
		h.service.Audit(r.Context(), authorization.AuditRecord{
			ActorUserID: principal.UserID, Action: "organization.membership.read",
			ResourceType: "organization_membership", Outcome: authorization.AuditDenied,
			RequestID: middleware.RequestIDFromContext(r.Context()),
		})
		writeAuthError(w, http.StatusNotFound, "not_found", "Resource not found.")
		return
	}
	h.service.Audit(r.Context(), authorization.AuditRecord{
		OrganizationID: tenant.OrganizationID, ActorUserID: tenant.UserID,
		Action: "organization.membership.read", ResourceType: "organization_membership",
		ResourceID: tenant.OrganizationID, Outcome: authorization.AuditSuccess,
		RequestID: middleware.RequestIDFromContext(r.Context()),
		Metadata:  map[string]string{"permission": string(authorization.PermissionOrganizationRead)},
	})
	writeAuthJSON(w, http.StatusOK, authorization.Membership{OrganizationID: tenant.OrganizationID, OrganizationName: tenant.OrganizationName, Role: tenant.Role})
}
