package client

// placement.go — Placement API reads (GPU capacity), ported from the operator's
// skyline-apiserver fork (skyline_apiserver/api/v1/placement.py): with nova
// PCI-in-placement, each physical GPU device is a resource provider carrying BOTH
// traits COMPUTE_MANAGED_PCI_DEVICE and CUSTOM_PCI_GPU; the GPU model is the remaining
// CUSTOM_PCI_<MODEL> trait (normalized to the pci alias form, e.g. CUSTOM_PCI_A100_80GB
// → "a100-80gb" — the same vocabulary pricing gpu_model and project GPU quota use);
// a device is in use when its RP has any allocation. 1 RP = 1 GPU device.

import (
	"context"
	"sort"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
)

// placementMicroversion pins the Placement API behavior the traits/allocations reads expect.
const placementMicroversion = "placement 1.10"

// GPUCapacity is one GPU model's cluster-wide device count for a region.
type GPUCapacity struct {
	Name  string `json:"name"`
	Total int    `json:"total"`
	InUse int    `json:"inUse"`
}

// placementGet performs a Placement API GET (token + microversion header attached).
func (c *Client) placementGet(ctx context.Context, url string, out any) error {
	_, err := c.provider.Request(ctx, "GET", url, &gophercloud.RequestOpts{
		JSONResponse: out,
		OkCodes:      []int{200},
		MoreHeaders:  map[string]string{"OpenStack-API-Version": placementMicroversion},
	})
	return err
}

// GPUInfo aggregates the region's GPU devices per model from Placement.
// Per-RP trait/allocation read failures skip that RP (best-effort).
func (c *Client) GPUInfo(ctx context.Context) ([]GPUCapacity, error) {
	base, err := c.EndpointURL("placement")
	if err != nil {
		return nil, err
	}
	base = strings.TrimRight(base, "/")
	var rps struct {
		ResourceProviders []struct {
			UUID string `json:"uuid"`
		} `json:"resource_providers"`
	}
	if err := c.placementGet(ctx, base+"/resource_providers", &rps); err != nil {
		return nil, err
	}
	type agg struct{ total, inUse int }
	byModel := map[string]*agg{}
	for _, rp := range rps.ResourceProviders {
		var tr struct {
			Traits []string `json:"traits"`
		}
		if err := c.placementGet(ctx, base+"/resource_providers/"+rp.UUID+"/traits", &tr); err != nil {
			continue
		}
		var managed, gpu bool
		model := ""
		for _, t := range tr.Traits {
			switch {
			case t == "COMPUTE_MANAGED_PCI_DEVICE":
				managed = true
			case t == "CUSTOM_PCI_GPU":
				gpu = true
			case strings.HasPrefix(t, "CUSTOM_PCI_") && model == "":
				model = strings.ReplaceAll(strings.ToLower(strings.TrimPrefix(t, "CUSTOM_PCI_")), "_", "-")
			}
		}
		if !managed || !gpu || model == "" {
			continue
		}
		a := byModel[model]
		if a == nil {
			a = &agg{}
			byModel[model] = a
		}
		a.total++
		var alloc struct {
			Allocations map[string]any `json:"allocations"`
		}
		if err := c.placementGet(ctx, base+"/resource_providers/"+rp.UUID+"/allocations", &alloc); err == nil && len(alloc.Allocations) > 0 {
			a.inUse++
		}
	}
	models := make([]string, 0, len(byModel))
	for m := range byModel {
		models = append(models, m)
	}
	sort.Strings(models)
	out := make([]GPUCapacity, 0, len(models))
	for _, m := range models {
		out = append(out, GPUCapacity{Name: m, Total: byModel[m].total, InUse: byModel[m].inUse})
	}
	return out, nil
}
