package providers

import (
	"testing"

	"github.com/menlocloud/stratos/internal/cloud"
)

func TestImagesToResources(t *testing.T) {
	in := []map[string]any{
		{"id": "img-1", "name": "pw-snap", "owner": "tenant-x", "image_type": "snapshot"},
		{"id": "img-2", "name": "cirros-public", "owner": "tenant-admin"}, // filtered — the dev125/187 leak class
		{"name": "no-id", "owner": "tenant-x"},                            // filtered — no id
	}
	got := imagesToResources(in, "RegionOne", "stratos-proj-1", "tenant-x")
	if len(got) != 1 {
		t.Fatalf("expected 1 (owner-filtered), got %d: %#v", len(got), got)
	}
	r := got[0]
	if r.Type != cloud.TypeImage || r.ExternalID != "img-1" || r.Region != "RegionOne" || r.ProjectID != "stratos-proj-1" {
		t.Errorf("bad resource (ProjectID must be the Stratos id, not the tenant): %#v", r)
	}
	if im, _ := r.Data["image"].(map[string]any); im == nil || im["id"] != "img-1" || im["image_type"] != "snapshot" {
		t.Errorf("data.image mismatch: %#v", r.Data)
	}
}

func TestImagesToResourcesNoOwnerProbe(t *testing.T) {
	// owner == "" (unscoped probe) disables the post-filter — never the syncjob path.
	in := []map[string]any{{"id": "img-1", "owner": "tenant-y"}}
	if got := imagesToResources(in, "RegionOne", "stratos-proj-1", ""); len(got) != 1 {
		t.Fatalf("empty owner must not filter: %#v", got)
	}
}
