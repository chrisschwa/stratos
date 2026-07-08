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

func TestIfaceFixedIPs(t *testing.T) {
	in := []any{
		map[string]any{"uuid": "net-1", "fixedIp": "10.0.0.10"},
		map[string]any{"uuid": "net-2"},               // no fixedIp → omitted
		map[string]any{"uuid": "", "fixedIp": "1.2.3.4"}, // no uuid → omitted
	}
	got := ifaceFixedIPs(in)
	if len(got) != 1 || got["net-1"] != "10.0.0.10" {
		t.Fatalf("ifaceFixedIPs = %#v", got)
	}
	if ifaceFixedIPs([]any{map[string]any{"uuid": "n"}}) != nil {
		t.Errorf("no fixed IPs → nil (not empty map)")
	}
}

func TestAddressPairs(t *testing.T) {
	in := []any{
		map[string]any{"ipAddress": "10.0.0.100"},
		map[string]any{"ipAddress": "10.0.0.0/24", "macAddress": "fa:16:3e:aa:bb:cc"},
		map[string]any{"macAddress": "no-ip"}, // no ipAddress → skipped
	}
	got := addressPairs(in)
	if len(got) != 2 || got[0].IPAddress != "10.0.0.100" || got[1].IPAddress != "10.0.0.0/24" || got[1].MACAddress != "fa:16:3e:aa:bb:cc" {
		t.Fatalf("addressPairs = %#v", got)
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
