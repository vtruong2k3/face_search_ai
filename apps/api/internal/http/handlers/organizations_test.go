package handlers

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/store"
)

type organizationsRepository struct {
	memberships []authorization.Membership
	byScope     map[string]authorization.Membership
	listUserID  string
	findUserID  string
}

func (r *organizationsRepository) ListMemberships(_ context.Context, userID string) ([]authorization.Membership, error) {
	r.listUserID = userID
	return r.memberships, nil
}

func (r *organizationsRepository) FindMembership(_ context.Context, userID, organizationID string) (authorization.Membership, error) {
	r.findUserID = userID
	membership, ok := r.byScope[organizationID]
	if !ok {
		return authorization.Membership{}, store.ErrNotFound
	}
	return membership, nil
}

func organizationRequest(method, target, userID string) *http.Request {
	request := httptest.NewRequest(method, target, nil)
	if userID != "" {
		request = request.WithContext(authorization.WithPrincipal(request.Context(), authorization.Principal{UserID: userID}))
	}
	return request
}

func TestOrganizationsListUsesAuthenticatedActor(t *testing.T) {
	repository := &organizationsRepository{memberships: []authorization.Membership{{OrganizationID: "org-one", OrganizationName: "One", Role: authorization.RoleViewer}}}
	handler := NewOrganizations(authorization.NewService(repository))
	request := organizationRequest(http.MethodGet, "/api/v1/organizations?userId=attacker&role=owner", "trusted-user")
	response := httptest.NewRecorder()
	handler.List(response, request)
	if response.Code != http.StatusOK || repository.listUserID != "trusted-user" {
		t.Fatalf("status=%d actor=%q body=%s", response.Code, repository.listUserID, response.Body.String())
	}
}

func TestOrganizationMembershipDoesNotEnumerateForeignScope(t *testing.T) {
	repository := &organizationsRepository{byScope: map[string]authorization.Membership{
		"own-org": {OrganizationID: "own-org", OrganizationName: "Own", Role: authorization.RoleViewer},
	}}
	handler := NewOrganizations(authorization.NewService(repository))

	ownRequest := organizationRequest(http.MethodGet, "/api/v1/organizations/own-org/membership?userId=attacker&role=owner", "trusted-user")
	ownRequest.SetPathValue("organizationId", "own-org")
	ownResponse := httptest.NewRecorder()
	handler.Membership(ownResponse, ownRequest)
	if ownResponse.Code != http.StatusOK || repository.findUserID != "trusted-user" {
		t.Fatalf("status=%d actor=%q body=%s", ownResponse.Code, repository.findUserID, ownResponse.Body.String())
	}

	var deniedBody string
	for _, organizationID := range []string{"foreign-org", "nonexistent-org"} {
		request := organizationRequest(http.MethodGet, "/api/v1/organizations/"+organizationID+"/membership", "trusted-user")
		request.SetPathValue("organizationId", organizationID)
		response := httptest.NewRecorder()
		handler.Membership(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("organization=%q status=%d", organizationID, response.Code)
		}
		if deniedBody == "" {
			deniedBody = response.Body.String()
		} else if response.Body.String() != deniedBody {
			t.Fatalf("enumerating responses differ: %q != %q", response.Body.String(), deniedBody)
		}
	}
	if deniedBody != "{\"code\":\"not_found\",\"message\":\"Resource not found.\"}\n" {
		t.Fatalf("unexpected denial body=%q", deniedBody)
	}
}

func TestOrganizationsRequireTrustedPrincipal(t *testing.T) {
	handler := NewOrganizations(authorization.NewService(&organizationsRepository{}))
	response := httptest.NewRecorder()
	handler.List(response, organizationRequest(http.MethodGet, "/api/v1/organizations?userId=attacker", ""))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
}
