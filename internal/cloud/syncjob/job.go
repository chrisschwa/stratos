// Package syncjob is the minimal live resource-sync driver: for each ENABLED project, for
// each CLOUD external service it is attached to, list the cloud's SERVER + PORT resources and
// upsert them into the `cloudResource` cache (stamped with the Stratos project id + region) so
// the metrics job + charge loop have a cache to read.
//
// This is the read-sync MVP — the full createOrUpdate dispatch (shouldBeDeleted /
// isNeededToUpdate / delete-of-vanished + the wasUserDeletedAfter guard) is a later slice.
// The CloudClient build is injectable (live default), so the walk is testable.
package syncjob

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
	"github.com/menlocloud/stratos/internal/cloud/providers"
	"github.com/menlocloud/stratos/internal/platform/externalservice"
	"github.com/menlocloud/stratos/internal/platform/project"
)

// ClientFactory builds a CloudClient for a service/region SCOPED to the project's tenant
// (externalProjectID). Scoping is REQUIRED — an admin/region-wide client lists every tenant's
// resources and would stamp them all onto this project (cross-tenant cache pollution + over-billing).
type ClientFactory func(ctx context.Context, es *externalservice.ExternalService, region, externalProjectID string) (*client.Client, error)

type Job struct {
	projects  *project.Repo
	services  *externalservice.Service
	cloud     *cloud.Repo
	clientFor ClientFactory
	now       func() time.Time
	log       *slog.Logger
}

func New(projects *project.Repo, services *externalservice.Service, cloudRepo *cloud.Repo, log *slog.Logger) *Job {
	if log == nil {
		log = slog.Default()
	}
	return &Job{
		projects: projects, services: services, cloud: cloudRepo,
		clientFor: func(ctx context.Context, es *externalservice.ExternalService, region, externalProjectID string) (*client.Client, error) {
			return client.New(ctx, es.ClientConfigForProject(region, externalProjectID))
		},
		now: func() time.Time { return time.Now().UTC() },
		log: log,
	}
}

func (j *Job) WithClientFactory(f ClientFactory) *Job { j.clientFor = f; return j }
func (j *Job) WithNow(now func() time.Time) *Job      { j.now = now; return j }

// SyncOne runs the sync for a single project — the admin POST /project/{id}/sync leg.
// serviceID == "" syncs every attached CLOUD service but only
// when the project is ENABLED (gated on isProjectActive); a non-blank
// serviceID syncs just that service with NO active gate (the scoped branch has none).
// ponytail: no distributed lock — the cron sync is idempotent-upsert so a concurrent run is
// harmless. Add a datastore lock (servicesSync-{p}-{es}, ~2min) if syncs ever fight.
func (j *Job) SyncOne(ctx context.Context, projectID, serviceID string) error {
	p, err := j.projects.FindByID(ctx, projectID)
	if err != nil {
		return err
	}
	if p == nil {
		return fmt.Errorf("project %s not found", projectID)
	}
	if serviceID == "" {
		if !p.IsEnabled() {
			return nil
		}
		j.syncProject(ctx, p, map[string]*externalservice.ExternalService{})
		return nil
	}
	es, err := j.services.Get(ctx, serviceID)
	if err != nil {
		return err
	}
	if es == nil {
		return fmt.Errorf("external service %s not found", serviceID)
	}
	if es.Type != externalservice.TypeCloud {
		return nil
	}
	j.syncService(ctx, p, es)
	return nil
}

// Run syncs every ENABLED project's CLOUD-service resources. Returns the total SERVER+PORT
// resources synced. Per-project / per-service failures are logged and skipped.
func (j *Job) Run(ctx context.Context) (int, error) {
	projects, err := j.projects.AllEnabled(ctx)
	if err != nil {
		return 0, err
	}
	esCache := map[string]*externalservice.ExternalService{}
	total := 0
	for i := range projects {
		total += j.syncProject(ctx, &projects[i], esCache)
	}
	return total, nil
}

func (j *Job) syncProject(ctx context.Context, p *project.Project, esCache map[string]*externalservice.ExternalService) int {
	count := 0
	for _, serviceID := range p.ServiceIDs() {
		es := esCache[serviceID]
		if es == nil {
			got, err := j.services.Get(ctx, serviceID)
			if err != nil {
				j.log.Error("syncjob: resolve external service", "project", p.ID, "serviceId", serviceID, "err", err)
				continue
			}
			esCache[serviceID] = got
			es = got
		}
		if es.Type != externalservice.TypeCloud {
			continue
		}
		count += j.syncService(ctx, p, es)
	}
	return count
}

// ProvidersFor is THE canonical per-project sync-provider set for a tenant-scoped client —
// shared by the cron walk, the admin project sync and the admin single-resource sync so every
// path reconciles with identical scoping/leak-guards.
//
// enabled gates the OPTIONAL service types by the provider's Services-tab toggles
// (config.services[slug][region]) — a disabled service is not listed at all, so e.g. a
// cloud whose key-manager is off no longer gets a barbican policy-denial logged per project
// per cycle. nil = everything on (single-resource sync targets an existing resource's type).
// Core compute/network/volume/image types are never gated: they back billing accrual.
func ProvidersFor(cc *client.Client, region, projectID, extProjID string, enabled func(slug string) bool) []providers.Provider {
	if enabled == nil {
		enabled = func(string) bool { return true }
	}
	ps := []providers.Provider{
		providers.NewServerProvider(cc, region, projectID),
		providers.NewPortProvider(cc, region, projectID),
		providers.NewVolumeProvider(cc, region, projectID),
		providers.NewFloatingIPProvider(cc, region, projectID),
		// neutron types: PROJECT-SCOPED (each List passes project_id + the mapper post-filters
		// tenant_id == externalProjectId — two-layer leak guard;
		// see providers/neutron_sync.go).
		providers.NewNetworkSyncProvider(cc, region, projectID, extProjID),
		providers.NewRouterSyncProvider(cc, region, projectID, extProjID),
		providers.NewSubnetSyncProvider(cc, region, projectID, extProjID),
		providers.NewSecurityGroupSyncProvider(cc, region, projectID, extProjID),
		// IMAGE: owner-filtered (glance list also returns other tenants' public/shared images —
		// the dev125/187 leak class; the image sync passes owner=externalProjectId).
		providers.NewImageSyncProvider(cc, region, projectID, extProjID),
		// Token-scoped (cinder/nova — no cross-tenant leak).
		providers.NewVolumeSnapshotProvider(cc, region, projectID),
		providers.NewServerGroupProvider(cc, region, projectID),
	}
	// Optional service types (token-scoped, no cross-tenant leak) — only when the service is
	// enabled for this provider+region.
	if enabled("load-balancer") {
		ps = append(ps, providers.NewLoadBalancerProvider(cc, region, projectID))
	}
	if enabled("key-manager") {
		ps = append(ps, providers.NewBarbicanSecretProvider(cc, region, projectID))
	}
	if enabled("object-store") {
		ps = append(ps, providers.NewBucketProvider(cc, region, projectID))
	}
	if enabled("dns") {
		ps = append(ps, providers.NewDNSZoneProvider(cc, region, projectID))
	}
	if enabled("orchestration") {
		ps = append(ps, providers.NewStackProvider(cc, region, projectID))
	}
	if enabled("sharev2") {
		ps = append(ps, providers.NewShareProvider(cc, region, projectID))
	}
	return ps
}

func (j *Job) syncService(ctx context.Context, p *project.Project, es *externalservice.ExternalService) int {
	count := 0
	extProjID := p.ExternalProjectID(es.ID)
	if extProjID == "" {
		// Not provisioned onto this service → no tenant to scope to. Skip rather than fall back to
		// an admin/region-wide client (which would pull every tenant's resources into this project).
		return 0
	}
	for _, region := range es.RegionNames() {
		cc, err := j.clientFor(ctx, es, region, extProjID)
		if err != nil {
			j.log.Error("syncjob: build cloud client", "project", p.ID, "serviceId", es.ID, "region", region, "err", err)
			continue
		}
		now := j.now()
		enabled := func(slug string) bool { return es.ServiceEnabledInRegion(slug, region) }
		for _, prov := range ProvidersFor(cc, region, p.ID, extProjID, enabled) {
			st, err := providers.Reconcile(ctx, prov, j.cloud, es.ID, now)
			if err != nil {
				j.log.Error("syncjob: reconcile", "project", p.ID, "serviceId", es.ID, "region", region, "type", prov.Type(), "err", err)
			}
			count += st.Created + st.Updated
		}
	}
	return count
}
