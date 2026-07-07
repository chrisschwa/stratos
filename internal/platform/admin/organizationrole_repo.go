package admin

// organizationrole_repo.go holds the OrganizationRole-specific repo methods the generic crud.go
// helpers do not cover: list-by-organization (OrganizationRoleRepository.findAllByOrganizationId) and
// the exists-by-org+name uniqueness check (existsByOrganizationIdAndName, an EXACT match — the name is
// already upper-cased by the service before the check).

import (
	"context"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// OrganizationRolesByOrganization returns every
// roleDefinition doc for the organization, as raw document (never nil). The caller shapes each into the
// OrganizationRoleDto.
func (r *Repo) OrganizationRolesByOrganization(ctx context.Context, organizationID string) ([]pgdoc.M, error) {
	out := []pgdoc.M{}
	if err := r.c(organizationRoleCollection).Find(ctx, pgdoc.M{"organizationId": organizationID}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// OrganizationRoleExistsByName reports true
// when a roleDefinition with the exact (organizationId, name) pair exists. The name is matched
// literally (case-sensitive) because the service upper-cases it before calling.
func (r *Repo) OrganizationRoleExistsByName(ctx context.Context, organizationID, name string) (bool, error) {
	return r.c(organizationRoleCollection).Exists(ctx, pgdoc.M{"organizationId": organizationID, "name": name})
}
