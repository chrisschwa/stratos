package providers

import (
	"context"
	"testing"

	"github.com/menlocloud/stratos/internal/cloud"
)

func TestIDKeyedProviderList(t *testing.T) {
	p := &idKeyedProvider{
		region: "RegionOne", projectID: "stratos-proj-1",
		typ: cloud.TypeStack, dataKey: "stack",
		list: func(ctx context.Context) ([]map[string]any, error) {
			return []map[string]any{
				{"id": "st-1", "stack_name": "pw-stack", "stack_status": "CREATE_COMPLETE"},
				{"stack_name": "no-id"}, // skipped — no id
			}, nil
		},
	}
	got, err := p.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 (no-id skipped), got %d: %#v", len(got), got)
	}
	r := got[0]
	if r.Type != cloud.TypeStack || r.ExternalID != "st-1" || r.Region != "RegionOne" || r.ProjectID != "stratos-proj-1" {
		t.Errorf("bad resource: %#v", r)
	}
	st, _ := r.Data["stack"].(map[string]any)
	if st == nil || st["stack_name"] != "pw-stack" {
		t.Errorf("data.stack mismatch: %#v", r.Data)
	}
	// CompareKeys = the dataKey (per-provider compare key).
	if p.CompareKeys()[0] != "stack" {
		t.Errorf("CompareKeys = %v", p.CompareKeys())
	}
}

func TestTokenProviderConstructors(t *testing.T) {
	// Each constructor returns a Provider with the right Type + a KeyedComparer with the right key.
	cases := []struct {
		p   Provider
		typ string
		key string
	}{
		{NewVolumeSnapshotProvider(nil, "R", "p"), cloud.TypeVolumeSnapshot, "volumeSnapshot"},
		{NewServerGroupProvider(nil, "R", "p"), cloud.TypeServerGroup, "serverGroup"},
		{NewStackProvider(nil, "R", "p"), cloud.TypeStack, "stack"},
		{NewShareProvider(nil, "R", "p"), cloud.TypeShare, "share"},
	}
	for _, c := range cases {
		if c.p.Type() != c.typ {
			t.Errorf("Type = %s, want %s", c.p.Type(), c.typ)
		}
		kc, ok := c.p.(KeyedComparer)
		if !ok || kc.CompareKeys()[0] != c.key {
			t.Errorf("%s: CompareKeys wrong (ok=%v): %#v", c.typ, ok, c.p)
		}
	}
}
