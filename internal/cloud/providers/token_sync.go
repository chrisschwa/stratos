package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// token_sync.go adds the remaining TOKEN-SCOPED read-sync providers (cinder volume-snapshot, nova
// server-group, heat stack, manila share) — closing their cache drift. SAFE without a project
// filter: cinder/nova/heat/manila scope to the project token, so each List returns only this
// project's resources (no cross-tenant leak, unlike neutron). externalId == the resource id; the
// Data shape mirrors the create/refreshResource shape so an unchanged resource produces no diff.
//
// ⚠ CODE + UNIT ONLY — NOT live-verified (shared dev region READ-ONLY this session). Pure mapping
// unit-tested; run `POST :8081/debug/run-sync` to exercise before relying on delete-of-vanished.
//
// DEFERRED (need care, not added here): KEYPAIR (externalId = "<name>_<userId>" — a sync can't
// reconstruct the userId → would never match the cached doc), IMAGE (glance lists public+shared+
// private of every tenant → needs an owner-visibility filter, the dev125/187 leak class), USER /
// identity (admin-scoped), KUBERNETES_CLUSTER (Magnum absent on the region).

// idKeyedProvider is a token-scoped sync provider: list raw maps → CloudResource{externalId=id,
// data={<dataKey>: <map>}}. dataKey is also the isNeededToUpdate compare key.
// idField defaults to "id".
type idKeyedProvider struct {
	cc        *client.Client
	region    string
	projectID string
	typ       string
	dataKey   string
	idField   string
	list      func(ctx context.Context) ([]map[string]any, error)
}

func (p *idKeyedProvider) Type() string          { return p.typ }
func (p *idKeyedProvider) CompareKeys() []string { return []string{p.dataKey} }
func (p *idKeyedProvider) ProjectID() string     { return p.projectID }

func (p *idKeyedProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	items, err := p.list(ctx)
	if err != nil {
		return nil, err
	}
	idField := p.idField
	if idField == "" {
		idField = "id"
	}
	out := make([]cloud.CloudResource, 0, len(items))
	for _, it := range items {
		id, _ := it[idField].(string)
		if id == "" {
			continue
		}
		out = append(out, cloud.CloudResource{
			Type: p.typ, ExternalID: id, Region: p.region, ProjectID: p.projectID,
			Data: map[string]any{p.dataKey: it},
		})
	}
	return out, nil
}

func NewVolumeSnapshotProvider(cc *client.Client, region, projectID string) Provider {
	return &idKeyedProvider{cc: cc, region: region, projectID: projectID,
		typ: cloud.TypeVolumeSnapshot, dataKey: "volumeSnapshot", list: cc.ListVolumeSnapshots}
}

func NewServerGroupProvider(cc *client.Client, region, projectID string) Provider {
	return &idKeyedProvider{cc: cc, region: region, projectID: projectID,
		typ: cloud.TypeServerGroup, dataKey: "serverGroup", list: cc.ListServerGroups}
}

func NewStackProvider(cc *client.Client, region, projectID string) Provider {
	return &idKeyedProvider{cc: cc, region: region, projectID: projectID,
		typ: cloud.TypeStack, dataKey: "stack", list: cc.ListStacks}
}

func NewShareProvider(cc *client.Client, region, projectID string) Provider {
	return &idKeyedProvider{cc: cc, region: region, projectID: projectID,
		typ: cloud.TypeShare, dataKey: "share", list: cc.ListShares}
}
