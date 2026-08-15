package postgres

import (
	"context"

	"github.com/face-search-ai/api/internal/domain/authorization"
)

type AuthorizationRepository struct {
	db *Store
}

func NewAuthorizationRepository(db *Store) *AuthorizationRepository {
	return &AuthorizationRepository{db: db}
}

func (r *AuthorizationRepository) ListMemberships(ctx context.Context, userID string) ([]authorization.Membership, error) {
	rows, err := r.db.Query(ctx, `
		SELECT m.organization_id, o.name, m.role
		FROM organization_memberships m
		JOIN organizations o ON o.id = m.organization_id
		JOIN users u ON u.id = m.user_id
		WHERE m.user_id = $1
		  AND m.status = 'active'
		  AND o.status = 'active'
		  AND u.status = 'active'
		ORDER BY o.name, m.organization_id`, userID)
	if err != nil {
		return nil, MapError(err)
	}
	defer rows.Close()

	memberships := make([]authorization.Membership, 0)
	for rows.Next() {
		var membership authorization.Membership
		if err := rows.Scan(&membership.OrganizationID, &membership.OrganizationName, &membership.Role); err != nil {
			return nil, MapError(err)
		}
		memberships = append(memberships, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, MapError(err)
	}
	return memberships, nil
}

func (r *AuthorizationRepository) FindMembership(ctx context.Context, userID, organizationID string) (authorization.Membership, error) {
	var membership authorization.Membership
	err := r.db.QueryRow(ctx, `
		SELECT m.organization_id, o.name, m.role
		FROM organization_memberships m
		JOIN organizations o ON o.id = m.organization_id
		JOIN users u ON u.id = m.user_id
		WHERE m.user_id = $1
		  AND m.organization_id = $2
		  AND m.status = 'active'
		  AND o.status = 'active'
		  AND u.status = 'active'`, userID, organizationID).Scan(
		&membership.OrganizationID,
		&membership.OrganizationName,
		&membership.Role,
	)
	if err != nil {
		return authorization.Membership{}, MapError(err)
	}
	return membership, nil
}

var _ authorization.Repository = (*AuthorizationRepository)(nil)
