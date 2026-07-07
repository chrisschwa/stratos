package providers

import (
	"testing"

	"github.com/menlocloud/stratos/internal/cloud"
)

// The mappers take (region, projectID, tenant): projectID STAMPS CloudResource.ProjectID (the
// Stratos internal id), tenant is the leak post-filter (the openstack tenant). The tests use
// distinct values to prove they're not conflated.

func TestNetworksToResources(t *testing.T) {
	in := []map[string]any{
		{"id": "net-1", "name": "pw-net", "tenant_id": "tenant-x"},
		{"id": "net-2", "name": "other-tenant", "tenant_id": "tenant-y"}, // filtered — wrong tenant
		{"name": "no-id", "tenant_id": "tenant-x"},                       // filtered — no id
	}
	got := networksToResources(in, "RegionOne", "stratos-proj-1", "tenant-x")
	if len(got) != 1 {
		t.Fatalf("expected 1 (tenant-filtered), got %d: %#v", len(got), got)
	}
	r := got[0]
	if r.Type != cloud.TypeNetwork || r.ExternalID != "net-1" || r.Region != "RegionOne" || r.ProjectID != "stratos-proj-1" {
		t.Errorf("bad resource (ProjectID must be the Stratos id, not the tenant): %#v", r)
	}
	if r.Data["networkName"] != "pw-net" {
		t.Errorf("networkName not lifted: %#v", r.Data)
	}
	if n, _ := r.Data["network"].(map[string]any); n == nil || n["id"] != "net-1" {
		t.Errorf("data.network mismatch: %#v", r.Data)
	}
}

func TestNetworksToResourcesProjectIDAlias(t *testing.T) {
	// Newer neutron returns project_id (not tenant_id) — the tenant filter must still match.
	in := []map[string]any{{"id": "net-1", "name": "pw-net", "project_id": "tenant-x"}}
	if got := networksToResources(in, "RegionOne", "stratos-proj-1", "tenant-x"); len(got) != 1 {
		t.Fatalf("project_id alias not matched: %#v", got)
	}
}

func TestRoutersToResources(t *testing.T) {
	in := []map[string]any{
		{"id": "rt-1", "name": "pw-router", "tenant_id": "tenant-x"},
		{"id": "rt-2", "name": "leak", "tenant_id": "tenant-y"}, // filtered
	}
	got := routersToResources(in, "RegionOne", "stratos-proj-1", "tenant-x")
	if len(got) != 1 || got[0].ExternalID != "rt-1" || got[0].Type != cloud.TypeRouter || got[0].ProjectID != "stratos-proj-1" {
		t.Fatalf("got %#v", got)
	}
	if got[0].Data["routerName"] != "pw-router" {
		t.Errorf("routerName not lifted: %#v", got[0].Data)
	}
	if rt, _ := got[0].Data["router"].(map[string]any); rt == nil || rt["id"] != "rt-1" {
		t.Errorf("data.router mismatch: %#v", got[0].Data)
	}
}

func TestSubnetsToResources(t *testing.T) {
	in := []map[string]any{
		{"id": "sub-1", "name": "pw-subnet", "tenant_id": "tenant-x"},
		{"id": "sub-2", "tenant_id": "tenant-y"}, // filtered
	}
	got := subnetsToResources(in, "RegionOne", "stratos-proj-1", "tenant-x")
	if len(got) != 1 || got[0].ExternalID != "sub-1" || got[0].Type != cloud.TypeSubnet {
		t.Fatalf("got %#v", got)
	}
	if s, _ := got[0].Data["subnet"].(map[string]any); s == nil || s["id"] != "sub-1" {
		t.Errorf("data.subnet mismatch: %#v", got[0].Data)
	}
}

func TestSecurityGroupsToResources(t *testing.T) {
	in := []map[string]any{
		{"id": "sg-1", "name": "pw-secgroup", "tenant_id": "tenant-x"},
		{"id": "sg-2", "name": "leak", "project_id": "tenant-y"}, // filtered
		{"name": "no-id", "tenant_id": "tenant-x"},               // filtered
	}
	got := securityGroupsToResources(in, "RegionOne", "stratos-proj-1", "tenant-x")
	if len(got) != 1 || got[0].ExternalID != "sg-1" || got[0].Type != cloud.TypeSecurityGroup {
		t.Fatalf("got %#v", got)
	}
	if sg, _ := got[0].Data["securityGroup"].(map[string]any); sg == nil || sg["name"] != "pw-secgroup" {
		t.Errorf("data.securityGroup mismatch: %#v", got[0].Data)
	}
}

// Empty tenant = unscoped admin probe → keep everything (never the syncjob path, but defined).
func TestBelongsToTenantAdminProbe(t *testing.T) {
	in := []map[string]any{
		{"id": "a", "tenant_id": "x"},
		{"id": "b", "project_id": "y"},
	}
	if got := networksToResources(in, "R", "stratos-proj-1", ""); len(got) != 2 {
		t.Fatalf("admin probe (empty tenant) must keep all, got %d", len(got))
	}
}
