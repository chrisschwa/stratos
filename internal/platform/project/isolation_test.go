package project

import (
	"testing"

	"github.com/menlocloud/stratos/internal/cloud"
)

// §5/§7: a cloud read/mutate/delete may only target a cache row owned by the acting project. A row
// owned by another project, or a nil row (a raw/uncached id), must be rejected.
func TestResourceOwnedBy_crossProjectDenied(t *testing.T) {
	owned := &cloud.CloudResource{ID: "r1", ExternalID: "ext-1", ProjectID: "proj-A"}
	if !resourceOwnedBy(owned, "proj-A") {
		t.Fatal("an owned resource must pass")
	}
	if resourceOwnedBy(owned, "proj-B") {
		t.Fatal("§5/§7: another project must not act on this resource")
	}
	if resourceOwnedBy(nil, "proj-A") {
		t.Fatal("§5/§7: a raw/uncached id (nil cache row) must be rejected, not acted on")
	}
}

// §27: the glance owner filter must apply in EVERY listImages branch, including the
// dataAssociatedTo (server-snapshots) branch — otherwise a caller reads another tenant's snapshots
// by naming their instance_uuid.
func TestImageVisibleTo_ownerEnforcedInAssocBranch(t *testing.T) {
	const tenant = "tenant-A"

	foreign := map[string]any{"owner": "tenant-B", "instance_uuid": "srv-1"}
	if imageVisibleTo(foreign, tenant, "srv-1") {
		t.Fatal("§27: owner check must gate out a foreign snapshot even when instance_uuid matches")
	}
	if imageVisibleTo(foreign, tenant, "") {
		t.Fatal("§27: owner check must gate out a foreign image in the my-images branch")
	}

	own := map[string]any{"owner": tenant, "instance_uuid": "srv-1"}
	if !imageVisibleTo(own, tenant, "srv-1") {
		t.Fatal("an own snapshot for the requested server must be visible")
	}
	// owner matches but the snapshot is for a different server → narrowed out by AND, not else-if.
	otherServer := map[string]any{"owner": tenant, "instance_uuid": "srv-9"}
	if imageVisibleTo(otherServer, tenant, "srv-1") {
		t.Fatal("§27: instance_uuid must narrow with AND")
	}
}

// §30: LB child actions must only accept listener/pool/member/monitor ids that belong to the target
// LB. lbChildSets builds that owned-id set; a child id from another LB must not be present.
func TestLbChildSets_rejectsForeignChildIDs(t *testing.T) {
	listeners := []map[string]any{{"id": "lis-own"}}
	pools := []map[string]any{{
		"id":               "pool-own",
		"healthmonitor_id": "mon-own",
		"members":          []map[string]any{{"id": "mem-own"}},
	}}
	kids := lbChildSets(listeners, pools)

	if !kids.listeners["lis-own"] || !kids.pools["pool-own"] || !kids.members["mem-own"] || !kids.monitors["mon-own"] {
		t.Fatal("this LB's own children must be in the set")
	}
	if kids.listeners["lis-foreign"] || kids.pools["pool-foreign"] || kids.members["mem-foreign"] || kids.monitors["mon-foreign"] {
		t.Fatal("§30: a child id from another LB must be rejected")
	}
}
