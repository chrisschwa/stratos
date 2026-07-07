package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// neutron_sync.go adds the project-scoped neutron read-sync providers (network/router/subnet/
// security_group) — the biggest remaining §5 cache-drift fix. These reconcile a project's
// neutron resources so a UI-deleted/vanished one is removed from the cache and a drifted one is
// refreshed.
//
// ⚠ LEAK GUARD (the dev160-161 lesson): a neutron token lists EVERY tenant's
// resources unless filtered. Two-layer defence:
//   1. the client List* methods pass `project_id` (neutron_list.go / ListSecurityGroups), AND
//   2. the pure mappers below post-filter `tenant_id|project_id == projectID`
//      (a `tenant_id == externalProjectId` second guard).
// projectID == "" (an unscoped admin probe) disables the post-filter — never the syncjob path,
// which always scopes to the project's externalProjectId.
//
// Each Data shape MIRRORS the create/refreshResource shape (cloud_writes.go / providers/write.go)
// so an unchanged resource produces no diff under Reconcile's whole-data compare:
//   NETWORK        → {"network": <neutron map>, "networkName": <name>}
//   ROUTER         → {"router":  <neutron map>, "routerName":  <name>}
//   SUBNET         → {"subnet":  <neutron map>}
//   SECURITY_GROUP → {"securityGroup": <neutron map>}
//
// ⚠ CODE + UNIT ONLY — NOT live-verified (the shared dev region is READ-ONLY this session). The
// pure mapping + tenant post-filter are unit-tested (neutron_sync_test.go); the leak guard MUST
// still be confirmed with a live `POST :8081/debug/run-sync` before relying on delete-of-vanished.

// belongsToTenant reports whether a raw neutron object belongs to the external openstack tenant
// (matches either the neutron `tenant_id` or `project_id` field). An empty tenant matches
// everything (an unscoped admin probe — never the syncjob path).
func belongsToTenant(obj map[string]any, tenant string) bool {
	if tenant == "" {
		return true
	}
	if t, _ := obj["tenant_id"].(string); t == tenant {
		return true
	}
	if p, _ := obj["project_id"].(string); p == tenant {
		return true
	}
	return false
}

// neutronSync carries BOTH ids: projectID (the Stratos internal project id) STAMPS each cached
// CloudResource.ProjectID (like every other provider); externalProjectID (the openstack tenant)
// is the leak post-filter only. Conflating them would stamp the wrong project id on the cache.
type neutronSync struct {
	cc                *client.Client
	region            string
	projectID         string
	externalProjectID string
}

// ProjectID exposes the Stratos project id for Reconcile's project-scoped delete-of-vanished scan.
func (s neutronSync) ProjectID() string { return s.projectID }

// --- NETWORK ---

type NetworkSyncProvider struct{ neutronSync }

func NewNetworkSyncProvider(cc *client.Client, region, projectID, externalProjectID string) *NetworkSyncProvider {
	return &NetworkSyncProvider{neutronSync{cc, region, projectID, externalProjectID}}
}

func (p *NetworkSyncProvider) Type() string { return cloud.TypeNetwork }

// CompareKeys: isNeededToUpdate checks "network" || "clusterInfo".
// The network sync doesn't populate clusterInfo (both sides absent → equal), but the key is kept
// for completeness.
func (p *NetworkSyncProvider) CompareKeys() []string { return []string{"network", "clusterInfo"} }

func (p *NetworkSyncProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	ns, err := p.cc.ListNetworksFull(ctx)
	if err != nil {
		return nil, err
	}
	return networksToResources(ns, p.region, p.projectID, p.externalProjectID), nil
}

func networksToResources(ns []map[string]any, region, projectID, tenant string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(ns))
	for _, n := range ns {
		id, _ := n["id"].(string)
		if id == "" || !belongsToTenant(n, tenant) {
			continue
		}
		name, _ := n["name"].(string)
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeNetwork, ExternalID: id, Region: region, ProjectID: projectID,
			Data: map[string]any{"network": n, "networkName": name},
		})
	}
	return out
}

// --- ROUTER ---

type RouterSyncProvider struct{ neutronSync }

func NewRouterSyncProvider(cc *client.Client, region, projectID, externalProjectID string) *RouterSyncProvider {
	return &RouterSyncProvider{neutronSync{cc, region, projectID, externalProjectID}}
}

func (p *RouterSyncProvider) Type() string          { return cloud.TypeRouter }
func (p *RouterSyncProvider) CompareKeys() []string { return []string{"router"} }

func (p *RouterSyncProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	rs, err := p.cc.ListRoutersFull(ctx)
	if err != nil {
		return nil, err
	}
	return routersToResources(rs, p.region, p.projectID, p.externalProjectID), nil
}

func routersToResources(rs []map[string]any, region, projectID, tenant string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(rs))
	for _, rt := range rs {
		id, _ := rt["id"].(string)
		if id == "" || !belongsToTenant(rt, tenant) {
			continue
		}
		name, _ := rt["name"].(string)
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeRouter, ExternalID: id, Region: region, ProjectID: projectID,
			Data: map[string]any{"router": rt, "routerName": name},
		})
	}
	return out
}

// --- SUBNET ---

type SubnetSyncProvider struct{ neutronSync }

func NewSubnetSyncProvider(cc *client.Client, region, projectID, externalProjectID string) *SubnetSyncProvider {
	return &SubnetSyncProvider{neutronSync{cc, region, projectID, externalProjectID}}
}

func (p *SubnetSyncProvider) Type() string          { return cloud.TypeSubnet }
func (p *SubnetSyncProvider) CompareKeys() []string { return []string{"subnet"} }

func (p *SubnetSyncProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	ss, err := p.cc.ListSubnetsFull(ctx)
	if err != nil {
		return nil, err
	}
	return subnetsToResources(ss, p.region, p.projectID, p.externalProjectID), nil
}

func subnetsToResources(ss []map[string]any, region, projectID, tenant string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(ss))
	for _, s := range ss {
		id, _ := s["id"].(string)
		if id == "" || !belongsToTenant(s, tenant) {
			continue
		}
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeSubnet, ExternalID: id, Region: region, ProjectID: projectID,
			Data: map[string]any{"subnet": s},
		})
	}
	return out
}

// --- SECURITY_GROUP ---

type SecurityGroupSyncProvider struct{ neutronSync }

func NewSecurityGroupSyncProvider(cc *client.Client, region, projectID, externalProjectID string) *SecurityGroupSyncProvider {
	return &SecurityGroupSyncProvider{neutronSync{cc, region, projectID, externalProjectID}}
}

func (p *SecurityGroupSyncProvider) Type() string          { return cloud.TypeSecurityGroup }
func (p *SecurityGroupSyncProvider) CompareKeys() []string { return []string{"securityGroup"} }

func (p *SecurityGroupSyncProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	sgs, err := p.cc.ListSecurityGroups(ctx)
	if err != nil {
		return nil, err
	}
	return securityGroupsToResources(sgs, p.region, p.projectID, p.externalProjectID), nil
}

func securityGroupsToResources(sgs []map[string]any, region, projectID, tenant string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(sgs))
	for _, sg := range sgs {
		id, _ := sg["id"].(string)
		if id == "" || !belongsToTenant(sg, tenant) {
			continue
		}
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeSecurityGroup, ExternalID: id, Region: region, ProjectID: projectID,
			Data: map[string]any{"securityGroup": sg},
		})
	}
	return out
}
