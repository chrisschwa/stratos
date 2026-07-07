package admin

import (
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
	"github.com/menlocloud/stratos/internal/cloud/providers"
	"github.com/menlocloud/stratos/internal/cloud/syncjob"
	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// cloudresourcemut.go serves the MUTATIONS of the cloud-resource surface
// (/api/v1/admin/cloud-resource): GET /{id}/sync and DELETE /{id}. Every read endpoint of this
// surface is ALREADY registered in handler.go (cloudResourceByID / cloudResourcesByUser /
// cloudResourcesByProject / cloudResourcesAll / the public-networks emptyCloudList stub) and is
// intentionally NOT re-registered here.
//
// Call graph:
//
//	sync(id)   = syncCloudResource(id)  [audited SYNC PROJECT]
//	           = getById → resolve ES + project → re-fetch the live OpenStack object → createOrUpdate
//	             the cache → return the refreshed entity.
//	delete(id) = get(id)-or-404 → delete(rc, id) — the REAL OpenStack delete via
//	             the provider write path + archive to cloudResourceHistory → 202 Accepted (empty).
//
// Both run LIVE here through the handler's own cloud deps (esSvc + cloudNew + cloud repo — no extra
// wiring). sync reuses syncjob.ProvidersFor + providers.Reconcile scoped to the resource's TYPE +
// project — a superset of a single-object refresh with the same end-state for the target (and
// the same delete-of-vanished the cron sync applies anyway). Types with no sync provider
// (identity resources like KEYPAIR — none has sync either) stay 501.
// cloudNew == nil (tests) → both degrade to the original 501 responses.
//
// Both gate on ADMIN_CLOUD_RESOURCE_MANAGE (the reads gate on ADMIN_CLOUD_RESOURCE_READ).

const cloudResourceManagePerm = "admin:cloud_resource:manage"

// routeCloudResourceMut registers ONLY the cloud-resource mutation routes. The {id} param name
// reuses the one handler.go already uses on /cloud-resource/{id} (chi requires a single param name
// at a given path position).
func (h *Handler) routeCloudResourceMut(r chi.Router) {
	r.Get("/cloud-resource/{id}/sync", h.cloudResourceSync)
	r.Delete("/cloud-resource/{id}", h.cloudResourceDelete)
}

// cloudResourceIDNotFound is the exact 404
// ("CloudResource with id %s not found", interpolated).
func cloudResourceIDNotFound(id string) *httpx.HTTPError {
	return httpx.NotFound(fmt.Sprintf("CloudResource with id %s not found", id))
}

// tenantClientFor builds a CloudClient scoped the way the ServiceContext resolves for a cached
// resource: the resource's externalService + region, admin-scoped to the owning project's
// externalProjectId (falling back to plain admin scope for identity resources with no project).
// Returns (nil, es, extProjID, nil) when the factory is unwired (tests).
func (h *Handler) tenantClientFor(w http.ResponseWriter, r *http.Request, res *cloud.CloudResource) (*client.Client, string, bool) {
	es, ok := h.externalServiceOr404(w, r, res.ServiceID)
	if !ok {
		return nil, "", false
	}
	region := res.Region
	if region == "" {
		region = h.serviceRegions(es)[0]
	}
	extProjID := ""
	if res.ProjectID != "" {
		proj, err := h.repo.FindDoc(r.Context(), projectCollection, res.ProjectID)
		if httpx.WriteError(w, err) {
			return nil, "", false
		}
		if proj != nil {
			extProjID = projectExternalID(proj, res.ServiceID)
		}
	}
	cfg := es.ClientConfig(region)
	if extProjID != "" {
		cfg = es.ClientConfigForProject(region, extProjID)
	}
	cc, err := h.cloudNew(r.Context(), cfg)
	if httpx.WriteError(w, err) {
		return nil, "", false
	}
	return cc, extProjID, true
}

// projectExternalID reads services[].externalProjectId for the given serviceId from a raw project doc.
func projectExternalID(proj pgdoc.M, serviceID string) string {
	arr, ok := proj["services"].(pgdoc.A)
	if !ok {
		return ""
	}
	for _, s := range arr {
		if sm, ok := s.(pgdoc.M); ok && sm["serviceId"] == serviceID {
			ext, _ := sm["externalProjectId"].(string)
			return ext
		}
	}
	return ""
}

// cloudResourceSync handles syncCloudResource(id): resolve the cached resource + its service +
// project, re-fetch from the live region and upsert the cache, return the refreshed doc (single).
// Implemented as a TYPE+project-scoped Reconcile (see file header).
func (h *Handler) cloudResourceSync(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, cloudResourceManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	res, err := h.cloud.FindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if res == nil {
		// getById → the same cloudResourceIdNotFound 404.
		httpx.WriteError(w, cloudResourceIDNotFound(id))
		return
	}
	if h.cloudNew == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"syncCloudResource not implemented"))
		return
	}
	cc, extProjID, ok := h.tenantClientFor(w, r, res)
	if !ok {
		return
	}
	var prov providers.Provider
	for _, p := range syncjob.ProvidersFor(cc, res.Region, res.ProjectID, extProjID) {
		if p.Type() == res.Type {
			prov = p
			break
		}
	}
	if prov == nil {
		// Identity / unsynced types (KEYPAIR, …) — no sync provider (none has sync either).
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			fmt.Sprintf("syncCloudResource for type %s not implemented", res.Type)))
		return
	}
	if _, err := providers.Reconcile(r.Context(), prov, h.cloud, res.ServiceID, time.Now().UTC()); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): SYNC PROJECT audit event.
	fresh, err := h.cloud.FindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if fresh == nil {
		// The live object vanished → the reconcile archived it; the trailing getById throws the
		// same not-found.
		httpx.WriteError(w, cloudResourceIDNotFound(id))
		return
	}
	httpx.OK(w, fresh)
}

// cloudResourceDelete handles deleteCloudResource: get(id)-or-404 → delete — the REAL
// OpenStack delete through the provider write path (WriteService.Delete also archives the cache
// row into cloudResourceHistory) → 202 Accepted with an empty body.
func (h *Handler) cloudResourceDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, cloudResourceManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	// get(id): cloudResourceRepository.findById(id).orElseThrow(notFound(...)).
	res, err := h.cloud.FindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if res == nil {
		httpx.WriteError(w, cloudResourceIDNotFound(id))
		return
	}
	if h.cloudNew == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"deleteCloudResource not implemented"))
		return
	}
	cc, _, ok := h.tenantClientFor(w, r, res)
	if !ok {
		return
	}
	ws := providers.NewWriteService(cc, h.cloud)
	if err := ws.Delete(r.Context(), res.ServiceID, res.ExternalID); err != nil {
		httpx.WriteError(w, httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError, err.Error()))
		return
	}
	// TODO(audit): auditAdmin(cloudResource, DELETE, PROJECT).
	w.WriteHeader(http.StatusAccepted)
}
