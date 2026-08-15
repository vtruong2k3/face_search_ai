package authorization

import (
	"context"
	"errors"
)

var (
	ErrUnauthenticated = errors.New("authentication required")
	ErrForbidden       = errors.New("authorization denied")
)

type Role string

const (
	RoleOwner  Role = "owner"
	RoleAdmin  Role = "admin"
	RoleEditor Role = "editor"
	RoleViewer Role = "viewer"
)

type Permission string

const (
	PermissionOrganizationRead   Permission = "organization:read"
	PermissionOrganizationManage Permission = "organization:manage"
	PermissionEventRead          Permission = "event:read"
	PermissionEventWrite         Permission = "event:write"
	PermissionPhotoRead          Permission = "photo:read"
	PermissionPhotoWrite         Permission = "photo:write"
	PermissionSearch             Permission = "search:execute"
	PermissionDownload           Permission = "download:execute"
)

var permissionsByRole = map[Role]map[Permission]struct{}{
	RoleOwner: permissionSet(
		PermissionOrganizationRead, PermissionOrganizationManage,
		PermissionEventRead, PermissionEventWrite,
		PermissionPhotoRead, PermissionPhotoWrite,
		PermissionSearch, PermissionDownload,
	),
	RoleAdmin: permissionSet(
		PermissionOrganizationRead, PermissionOrganizationManage,
		PermissionEventRead, PermissionEventWrite,
		PermissionPhotoRead, PermissionPhotoWrite,
		PermissionSearch, PermissionDownload,
	),
	RoleEditor: permissionSet(
		PermissionOrganizationRead,
		PermissionEventRead, PermissionEventWrite,
		PermissionPhotoRead, PermissionPhotoWrite,
		PermissionSearch, PermissionDownload,
	),
	RoleViewer: permissionSet(
		PermissionOrganizationRead,
		PermissionEventRead, PermissionPhotoRead,
		PermissionSearch, PermissionDownload,
	),
}

func permissionSet(permissions ...Permission) map[Permission]struct{} {
	set := make(map[Permission]struct{}, len(permissions))
	for _, permission := range permissions {
		set[permission] = struct{}{}
	}
	return set
}

func Allows(role Role, permission Permission) bool {
	permissions, knownRole := permissionsByRole[role]
	if !knownRole {
		return false
	}
	_, allowed := permissions[permission]
	return allowed
}

type Membership struct {
	OrganizationID   string `json:"organizationId"`
	OrganizationName string `json:"organizationName"`
	Role             Role   `json:"role"`
}

type Principal struct {
	UserID string
}

type TenantPrincipal struct {
	UserID           string
	OrganizationID   string
	OrganizationName string
	Role             Role
}

type principalContextKey struct{}
type tenantPrincipalContextKey struct{}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok && principal.UserID != ""
}

func WithTenantPrincipal(ctx context.Context, principal TenantPrincipal) context.Context {
	return context.WithValue(ctx, tenantPrincipalContextKey{}, principal)
}

func TenantPrincipalFromContext(ctx context.Context) (TenantPrincipal, bool) {
	principal, ok := ctx.Value(tenantPrincipalContextKey{}).(TenantPrincipal)
	return principal, ok && principal.UserID != "" && principal.OrganizationID != ""
}

type Repository interface {
	ListMemberships(context.Context, string) ([]Membership, error)
	FindMembership(context.Context, string, string) (Membership, error)
}

type Service struct {
	repository Repository
	auditor    Auditor
}

func NewService(repository Repository) *Service {
	return &Service{repository: repository}
}

func NewServiceWithAuditor(repository Repository, auditor Auditor) *Service {
	return &Service{repository: repository, auditor: auditor}
}

func (s *Service) ListMemberships(ctx context.Context, userID string) ([]Membership, error) {
	if userID == "" {
		return nil, ErrUnauthenticated
	}
	return s.repository.ListMemberships(ctx, userID)
}

func (s *Service) Audit(ctx context.Context, record AuditRecord) {
	if s.auditor != nil {
		_ = s.auditor.WriteAudit(ctx, SafeAuditRecord(record))
	}
}

func (s *Service) Authorize(ctx context.Context, userID, organizationID string, permission Permission) (TenantPrincipal, error) {
	if userID == "" {
		return TenantPrincipal{}, ErrUnauthenticated
	}
	if organizationID == "" {
		return TenantPrincipal{}, ErrForbidden
	}
	membership, err := s.repository.FindMembership(ctx, userID, organizationID)
	if err != nil || !Allows(membership.Role, permission) {
		return TenantPrincipal{}, ErrForbidden
	}
	return TenantPrincipal{UserID: userID, OrganizationID: membership.OrganizationID, OrganizationName: membership.OrganizationName, Role: membership.Role}, nil
}
