package client

import (
	"context"
	"strings"
)

// neutron_list.go adds READ-ONLY, PROJECT-SCOPED "full" list calls for the neutron resource
// types the sync job reconciles (network/router/subnet) — returning the raw neutron object maps
// (the same snake_case shape the per-id Get* methods return, so a synced resource and a
// refreshResource read produce the same `data.*` sub-doc → no spurious diff/churn).
//
// ⚠ CRITICAL — every neutron list passes `project_id`. A neutron ADMIN token lists EVERY
// tenant's resources unless filtered (the dev160-161 cross-tenant leak). The per-type sync
// lists with an admin-scoped client filtered by `project_id=externalProjectId`
// AND post-filters `tenant_id == externalProjectId` as a second guard — the provider mappers
// (neutron_sync.go) replicate that post-filter. Empty projectID → unfiltered (admin probe only).

// neutronListFull GETs `/v2.0/{collection}[?project_id=…]` and returns the inner object array
// under `key` (e.g. networks/{networks}, routers/{routers}, subnets/{subnets}).
func (c *Client) neutronListFull(ctx context.Context, collection, key string) ([]map[string]any, error) {
	base, err := c.EndpointURL("network")
	if err != nil {
		return nil, err
	}
	url := strings.TrimRight(base, "/") + "/v2.0/" + collection
	if c.projectID != "" {
		url += "?project_id=" + c.projectID
	}
	var resp map[string][]map[string]any
	if err := c.Do(ctx, "GET", url, nil, &resp); err != nil {
		return nil, err
	}
	return resp[key], nil
}

// ListNetworksFull returns the project's Neutron networks as raw maps (mirrors GetNetwork's shape).
func (c *Client) ListNetworksFull(ctx context.Context) ([]map[string]any, error) {
	return c.neutronListFull(ctx, "networks", "networks")
}

// ListRoutersFull returns the project's Neutron routers as raw maps (mirrors GetRouter's shape).
func (c *Client) ListRoutersFull(ctx context.Context) ([]map[string]any, error) {
	return c.neutronListFull(ctx, "routers", "routers")
}

// ListSubnetsFull returns the project's Neutron subnets as raw maps.
func (c *Client) ListSubnetsFull(ctx context.Context) ([]map[string]any, error) {
	return c.neutronListFull(ctx, "subnets", "subnets")
}
