package authorization

import (
	"context"
	"errors"
	"testing"
)

type fakeRepository struct {
	memberships []Membership
	membership  Membership
	err         error
}

func (f *fakeRepository) ListMemberships(context.Context, string) ([]Membership, error) {
	return f.memberships, f.err
}
func (f *fakeRepository) FindMembership(context.Context, string, string) (Membership, error) {
	return f.membership, f.err
}

func TestRolePermissionMatrix(t *testing.T) {
	tests := []struct {
		role       Role
		permission Permission
		allowed    bool
	}{
		{RoleOwner, PermissionOrganizationManage, true},
		{RoleAdmin, PermissionOrganizationManage, true},
		{RoleEditor, PermissionOrganizationManage, false},
		{RoleEditor, PermissionEventWrite, true},
		{RoleViewer, PermissionEventWrite, false},
		{RoleViewer, PermissionEventRead, true},
		{RoleViewer, PermissionPhotoWrite, false},
		{RoleViewer, PermissionSearch, true},
		{Role("unknown"), PermissionOrganizationRead, false},
		{RoleOwner, Permission("unknown"), false},
	}
	for _, test := range tests {
		if got := Allows(test.role, test.permission); got != test.allowed {
			t.Errorf("Allows(%q, %q) = %v, want %v", test.role, test.permission, got, test.allowed)
		}
	}
}

func TestAuthorizeReturnsTenantPrincipalForAllowedMembership(t *testing.T) {
	service := NewService(&fakeRepository{membership: Membership{OrganizationID: "org-1", Role: RoleEditor}})
	principal, err := service.Authorize(context.Background(), "user-1", "org-1", PermissionEventWrite)
	if err != nil {
		t.Fatal(err)
	}
	if principal.UserID != "user-1" || principal.OrganizationID != "org-1" || principal.Role != RoleEditor {
		t.Fatalf("principal = %#v", principal)
	}
}

func TestAuthorizeFailsClosed(t *testing.T) {
	tests := []struct {
		name       string
		userID     string
		orgID      string
		membership Membership
		repoErr    error
		permission Permission
		want       error
	}{
		{"missing identity", "", "org-1", Membership{}, nil, PermissionEventRead, ErrUnauthenticated},
		{"missing organization", "user-1", "", Membership{}, nil, PermissionEventRead, ErrForbidden},
		{"membership absent", "user-1", "org-2", Membership{}, errors.New("not found"), PermissionEventRead, ErrForbidden},
		{"unknown role", "user-1", "org-1", Membership{OrganizationID: "org-1", Role: Role("unknown")}, nil, PermissionEventRead, ErrForbidden},
		{"insufficient role", "user-1", "org-1", Membership{OrganizationID: "org-1", Role: RoleViewer}, nil, PermissionEventWrite, ErrForbidden},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			service := NewService(&fakeRepository{membership: test.membership, err: test.repoErr})
			_, err := service.Authorize(context.Background(), test.userID, test.orgID, test.permission)
			if !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestPrincipalContextsAreTypedAndIndependent(t *testing.T) {
	ctx := WithPrincipal(context.Background(), Principal{UserID: "user-1"})
	ctx = WithTenantPrincipal(ctx, TenantPrincipal{UserID: "user-1", OrganizationID: "org-1", Role: RoleAdmin})
	principal, ok := PrincipalFromContext(ctx)
	if !ok || principal.UserID != "user-1" {
		t.Fatalf("principal = %#v, %v", principal, ok)
	}
	tenant, ok := TenantPrincipalFromContext(ctx)
	if !ok || tenant.OrganizationID != "org-1" || tenant.Role != RoleAdmin {
		t.Fatalf("tenant principal = %#v, %v", tenant, ok)
	}
	if _, ok := PrincipalFromContext(context.Background()); ok {
		t.Fatal("empty context contained a principal")
	}
}
