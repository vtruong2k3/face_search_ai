package postgres

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/face-search-ai/api/internal/domain/authorization"
	"github.com/face-search-ai/api/internal/store"
)

func TestIntegrationAuthorizationRepositoryIsolatesTenants(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var userOne, userTwo, orgOne, orgTwo string
	if err := db.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,'hash') RETURNING id", "authorization-one@example.test").Scan(&userOne); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,'hash') RETURNING id", "authorization-two@example.test").Scan(&userTwo); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES($1,$2) RETURNING id", "Authorization One", "authorization-one").Scan(&orgOne); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES($1,$2) RETURNING id", "Authorization Two", "authorization-two").Scan(&orgTwo); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO organization_memberships(organization_id,user_id,role) VALUES($1,$2,'owner'),($3,$4,'viewer')", orgOne, userOne, orgTwo, userTwo); err != nil {
		t.Fatal(err)
	}

	repo := NewAuthorizationRepository(db)
	memberships, err := repo.ListMemberships(ctx, userOne)
	if err != nil {
		t.Fatal(err)
	}
	if len(memberships) != 1 || memberships[0].OrganizationID != orgOne || memberships[0].Role != authorization.RoleOwner {
		t.Fatalf("user one memberships = %#v", memberships)
	}
	if _, err := repo.FindMembership(ctx, userOne, orgTwo); !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-tenant lookup error = %v", err)
	}
	if membership, err := repo.FindMembership(ctx, userTwo, orgTwo); err != nil || membership.Role != authorization.RoleViewer {
		t.Fatalf("own membership = %#v, %v", membership, err)
	}
}

func TestIntegrationAuditRepositoryPersistsSafeRecord(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var userID, organizationID string
	if err := db.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,'hash') RETURNING id", "audit-authorization@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Audit Authorization','audit-authorization') RETURNING id").Scan(&organizationID); err != nil {
		t.Fatal(err)
	}
	record := authorization.AuditRecord{
		OrganizationID: organizationID,
		ActorUserID:    userID,
		Action:         "organization.membership.read",
		ResourceType:   "organization_membership",
		ResourceID:     organizationID,
		Outcome:        authorization.AuditSuccess,
		RequestID:      "request-safe-123",
		Metadata:       map[string]string{"permission": "organization:read"},
	}
	if err := NewAuditRepository(db).WriteAudit(ctx, record); err != nil {
		t.Fatal(err)
	}
	var action, outcome, requestID, metadata string
	if err := db.QueryRow(ctx, `SELECT action, outcome, request_id, metadata::text FROM audit_records WHERE organization_id=$1 AND actor_user_id=$2 ORDER BY id DESC LIMIT 1`, organizationID, userID).Scan(&action, &outcome, &requestID, &metadata); err != nil {
		t.Fatal(err)
	}
	if action != record.Action || outcome != string(record.Outcome) || requestID != record.RequestID || metadata != `{"permission": "organization:read"}` {
		t.Fatalf("audit action=%q outcome=%q request=%q metadata=%q", action, outcome, requestID, metadata)
	}
}

func TestIntegrationAuthorizationRepositoryRejectsInactiveScope(t *testing.T) {
	databaseURL := os.Getenv("API_POSTGRES_INTEGRATION_URL")
	if databaseURL == "" {
		t.Skip("API_POSTGRES_INTEGRATION_URL is not set")
	}
	ctx := context.Background()
	db, err := Open(ctx, databaseURL, 4, 1)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	var userID, disabledMembershipOrg, suspendedOrg string
	if err := db.QueryRow(ctx, "INSERT INTO users(email,password_hash) VALUES($1,'hash') RETURNING id", "authorization-inactive@example.test").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "INSERT INTO organizations(name,slug) VALUES('Disabled Membership','disabled-membership') RETURNING id").Scan(&disabledMembershipOrg); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, "INSERT INTO organizations(name,slug,status) VALUES('Suspended Organization','suspended-organization','suspended') RETURNING id").Scan(&suspendedOrg); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, "INSERT INTO organization_memberships(organization_id,user_id,role,status) VALUES($1,$2,'admin','disabled'),($3,$2,'owner','active')", disabledMembershipOrg, userID, suspendedOrg); err != nil {
		t.Fatal(err)
	}

	repo := NewAuthorizationRepository(db)
	memberships, err := repo.ListMemberships(ctx, userID)
	if err != nil || len(memberships) != 0 {
		t.Fatalf("inactive memberships = %#v, %v", memberships, err)
	}
	for _, orgID := range []string{disabledMembershipOrg, suspendedOrg} {
		if _, err := repo.FindMembership(ctx, userID, orgID); !errors.Is(err, store.ErrNotFound) {
			t.Fatalf("inactive organization %s error = %v", orgID, err)
		}
	}
}
