package admin

import (
	"context"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// externalservicemut_repo.go holds the domain repo method backing the ExternalService DELETE in-use
// guards. The mutations themselves reuse the crud.go helpers (FindDoc / ReplaceDoc / DeleteDoc); the
// only thing crud.go lacks is the existence probe the three services run before a delete.

// externalServiceInUse runs the three in-use checks before deleting an
// external service, all of the same shape:
//
//	projectService.isExternalServiceInUse(id)        = exists(project where services.serviceId == id)
//	userService.isExternalServiceInUse(id)           = exists(user where services.serviceId == id)
//	cloudResourceService.isExternalServiceInUse(id)  = exists(cloudResource where serviceId == id)
//
// The cloudResource probe keys off the top-level `serviceId` field (existsByServiceId), while the
// project/user probes key off the embedded `services` array (some element's serviceId — array
// containment, the explicit form of the old implicit `services.serviceId` match). The collection
// name selects which filter to apply.
func (r *Repo) externalServiceInUse(ctx context.Context, collection, externalServiceID string) (bool, error) {
	filter := pgdoc.M{"services": pgdoc.M{"$contains": pgdoc.M{"serviceId": externalServiceID}}}
	if collection == "cloudResource" {
		filter = pgdoc.M{"serviceId": externalServiceID}
	}
	return r.c(collection).Exists(ctx, filter)
}
