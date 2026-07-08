package project

import (
	"testing"

	"github.com/menlocloud/stratos/internal/cloud/client"
)

func TestChooseExternalNetwork(t *testing.T) {
	nets := []client.Network{{ID: "a"}, {ID: "b"}, {ID: "c"}}

	// Router match present in the allowed set → that one wins over any random pick.
	if got := chooseExternalNetwork(nets, "b"); got != "b" {
		t.Errorf("router-match: got %q, want b", got)
	}
	// Router match NOT in the set → fall through to a pick (here any of a/b/c).
	if got := chooseExternalNetwork(nets, "zzz"); got != "a" && got != "b" && got != "c" {
		t.Errorf("router-miss: got %q, want one of a/b/c", got)
	}
	// Single allowed network → deterministic, ignores the (absent) router match.
	if got := chooseExternalNetwork([]client.Network{{ID: "only"}}, ""); got != "only" {
		t.Errorf("single: got %q, want only", got)
	}
	// Empty set → "" so the caller fails the create as a missing pool.
	if got := chooseExternalNetwork(nil, "b"); got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
}

func TestFirstRouterExternalNet(t *testing.T) {
	routers := []map[string]any{
		{"id": "r1"}, // no gateway
		{"id": "r2", "external_gateway_info": map[string]any{"network_id": ""}}, // gateway, no net
		{"id": "r3", "external_gateway_info": map[string]any{"network_id": "ext-9"}},
	}
	if got := firstRouterExternalNet(routers); got != "ext-9" {
		t.Errorf("got %q, want ext-9", got)
	}
	if got := firstRouterExternalNet([]map[string]any{{"id": "r1"}}); got != "" {
		t.Errorf("no-gateway: got %q, want empty", got)
	}
}
