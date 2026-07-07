package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/snapshots"
	"github.com/gophercloud/gophercloud/v2/openstack/compute/v2/servergroups"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/shares"
)

// list_sync.go adds READ-ONLY list calls for the TOKEN-SCOPED cloud types the sync job reconciles
// beyond server/port/volume/fip/lb + the niche/neutron sets: cinder volume snapshots, nova server
// groups, heat stacks, manila shares. These scope to the project TOKEN (cinder/nova/heat/manila
// return only this project's resources — no cross-tenant leak, unlike neutron), so no project_id
// filter is needed. Each returns the same per-item `toMap` shape the create/Get* methods produce,
// so a synced resource and a refreshResource read yield identical `data.*` sub-docs (no churn).

// ListVolumeSnapshots returns the project's Cinder volume snapshots (data.volumeSnapshot shape).
func (c *Client) ListVolumeSnapshots(ctx context.Context) ([]map[string]any, error) {
	bc, err := c.blockStorage()
	if err != nil {
		return nil, err
	}
	pages, err := snapshots.List(bc, snapshots.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ss, err := snapshots.ExtractSnapshots(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ss))
	for i := range ss {
		out = append(out, toMap(ss[i]))
	}
	return out, nil
}

// ListServerGroups returns the project's Nova server groups (data.serverGroup shape).
func (c *Client) ListServerGroups(ctx context.Context) ([]map[string]any, error) {
	cc, err := c.compute()
	if err != nil {
		return nil, err
	}
	pages, err := servergroups.List(cc, servergroups.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	gs, err := servergroups.ExtractServerGroups(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(gs))
	for i := range gs {
		out = append(out, toMap(gs[i]))
	}
	return out, nil
}

// ListStacks returns the project's Heat stacks (data.stack shape). The list view carries id +
// stack_name (Heat is name+id keyed) which the cache/refresh need.
func (c *Client) ListStacks(ctx context.Context) ([]map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	pages, err := stacks.List(hc, stacks.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ls, err := stacks.ExtractStacks(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ls))
	for i := range ls {
		out = append(out, toMap(ls[i]))
	}
	return out, nil
}

// ListShares returns the project's Manila shares (data.share shape).
func (c *Client) ListShares(ctx context.Context) ([]map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	pages, err := shares.ListDetail(sc, shares.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ls, err := shares.ExtractShares(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ls))
	for i := range ls {
		out = append(out, toMap(ls[i]))
	}
	return out, nil
}
