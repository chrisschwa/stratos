package admin

// discovery.go wires the LIVE keystone service/region discovery for a cloud provider. Creating a
// provider from the admin UI persists only the operator-supplied connection fields — it does NOT
// populate config.regions / config.services (those are normally filled by the post-create
// provisioning step, which a UI-created provider skips). Without them the client dashboard menu +
// create-resource Location dropdown are
// empty, so a UI-created provider cannot provision anything. discoverCatalog reads the token's
// service catalog and fills the two maps in the exact shape the seeded provider uses
// (deploy/seed/external-service-dev.json).

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// catalogTypeToSlug maps a keystone service-catalog `type` to the config.services slug the client
// dashboard menu keys off. Types absent here (identity, placement, …) are not provisionable and
// contribute only their region. Several cinder/manila type aliases collapse onto one slug.
var catalogTypeToSlug = map[string]string{
	"compute":            "compute",
	"network":            "network",
	"image":              "image",
	"volumev3":           "volumev3",
	"volumev2":           "volumev3",
	"volume":             "volumev3",
	"block-storage":      "volumev3",
	"metric":             "metric",
	"dns":                "dns",
	"key-manager":        "key-manager",
	"object-store":       "object-store",
	"load-balancer":      "load-balancer",
	"orchestration":      "orchestration",
	"sharev2":            "sharev2",
	"share":              "sharev2",
	"shared-file-system": "sharev2",
	"container-infra":    "container-infra",
}

// discoverCatalog authenticates with the service's stored admin creds and reads the token's service
// catalog, deriving:
//   - regions  = every distinct endpoint region → {name, country:"", displayName:""}
//   - services = catalogTypeToSlug[type] → {region → true} for each provisionable service's endpoints
//
// Empty maps + error when the cloud client is unavailable or auth fails; the caller decides whether
// that is fatal (the /discover endpoint 400s; create swallows it).
func (h *Handler) discoverCatalog(ctx context.Context, esID string) (regions, services pgdoc.M, err error) {
	regions, services = pgdoc.M{}, pgdoc.M{}
	if h.esSvc == nil || h.cloudNew == nil {
		return regions, services, httpx.BadRequest("Cloud client is not configured on this deployment")
	}
	es, err := h.esSvc.Get(ctx, esID)
	if err != nil || es == nil {
		return regions, services, httpx.BadRequest("Service not found: " + esID)
	}
	cc, err := h.cloudNew(ctx, es.ClientConfig(h.region))
	if err != nil {
		return regions, services, httpx.BadRequest("OpenStack authentication failed: " + err.Error())
	}
	for _, entry := range keystoneArray(ctx, cc, es.IdentityURL(), "/auth/catalog", "catalog") {
		m, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		slug := catalogTypeToSlug[strings.ToLower(str(m["type"]))]
		endpoints, _ := m["endpoints"].([]any)
		for _, ep := range endpoints {
			em, ok := ep.(map[string]any)
			if !ok {
				continue
			}
			region := str(em["region"])
			if region == "" {
				region = str(em["region_id"])
			}
			if region == "" {
				continue
			}
			if _, seen := regions[region]; !seen {
				regions[region] = pgdoc.M{"name": region, "country": "", "displayName": ""}
			}
			if slug == "" {
				continue // region-only: not a provisionable service type
			}
			svc, _ := services[slug].(pgdoc.M)
			if svc == nil {
				svc = pgdoc.M{}
				services[slug] = svc
			}
			svc[region] = true
		}
	}
	return regions, services, nil
}

// mergeDiscovered folds the discovered regions/services onto the stored config ADDITIVELY: it adds
// regions and service×region toggles that are absent, and never overwrites an existing entry (so an
// operator's manual enable/disable in the Services tab survives a re-sync).
func mergeDiscovered(cfg pgdoc.M, regions, services pgdoc.M) {
	r := ensureMap(cfg, "regions")
	for k, v := range regions {
		if _, ok := r[k]; !ok {
			r[k] = v
		}
	}
	s := ensureMap(cfg, "services")
	for slug, regsAny := range services {
		regs, _ := regsAny.(pgdoc.M)
		existing := ensureMap(s, slug)
		for region, on := range regs {
			if _, ok := existing[region]; !ok {
				existing[region] = on
			}
		}
	}
}

// externalServiceDiscover handles POST /service/{id}/discover: re-read the provider, run the live
// catalog discovery, merge the regions/services onto its config, persist, and return the shaped doc.
// Gated ADMIN_SERVICE_MANAGE. Auth/read failure → 400 (this is an on-demand connection action).
func (h *Handler) externalServiceDiscover(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, externalServiceManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	doc, err := h.repo.FindDoc(r.Context(), externalServiceCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if doc == nil {
		httpx.WriteError(w, serviceNotFoundErr(id))
		return
	}
	regions, services, derr := h.discoverCatalog(r.Context(), id)
	if httpx.WriteError(w, derr) {
		return
	}
	mergeDiscovered(ensureConfig(doc), regions, services)
	if err := h.repo.ReplaceDoc(r.Context(), externalServiceCollection, id, doc); httpx.WriteError(w, err) {
		return
	}
	shapeExternalService(doc)
	httpx.OK(w, doc)
}

// enrichNewServiceFromCloud is the best-effort auto-discovery run right after a UI create: it merges
// the discovered regions/services onto the just-inserted doc's config and persists them. Any failure
// (cloud unreachable, bad creds) is swallowed — the create itself already succeeded and the operator
// can re-run discovery via the Sync button. Returns whether the doc was enriched.
//
//nolint:unused // called from externalServiceCreate
func (h *Handler) enrichNewServiceFromCloud(ctx context.Context, doc pgdoc.M, id string) bool {
	regions, services, err := h.discoverCatalog(ctx, id)
	if err != nil || (len(regions) == 0 && len(services) == 0) {
		return false
	}
	mergeDiscovered(ensureConfig(doc), regions, services)
	if err := h.repo.ReplaceDoc(ctx, externalServiceCollection, id, doc); err != nil {
		return false
	}
	return true
}
