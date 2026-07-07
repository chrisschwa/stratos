package providers

import "testing"

func TestAllocationPoolsParse(t *testing.T) {
	in := []any{
		map[string]any{"start": "10.0.0.10", "end": "10.0.0.20"},
		map[string]any{"start": "10.0.0.30", "end": "10.0.0.40"},
		"not-a-map", // skipped
	}
	got := allocationPools(in)
	if len(got) != 2 || got[0].Start != "10.0.0.10" || got[0].End != "10.0.0.20" || got[1].Start != "10.0.0.30" {
		t.Fatalf("allocationPools = %#v", got)
	}
	if allocationPools(nil) == nil || len(allocationPools(nil)) != 0 {
		t.Errorf("nil → empty slice")
	}
}

func TestHostRoutesParse(t *testing.T) {
	in := []any{map[string]any{"destination": "0.0.0.0/0", "nexthop": "10.0.0.1"}}
	got := hostRoutes(in)
	if len(got) != 1 || got[0].DestinationCIDR != "0.0.0.0/0" || got[0].NextHop != "10.0.0.1" {
		t.Fatalf("hostRoutes = %#v", got)
	}
}

func TestMboolPtr(t *testing.T) {
	if mboolPtr(map[string]any{}, "x") != nil {
		t.Errorf("absent → nil")
	}
	if p := mboolPtr(map[string]any{"x": false}, "x"); p == nil || *p != false {
		t.Errorf("present false → &false, got %v", p)
	}
	if p := mboolPtr(map[string]any{"x": true}, "x"); p == nil || *p != true {
		t.Errorf("present true → &true, got %v", p)
	}
}
