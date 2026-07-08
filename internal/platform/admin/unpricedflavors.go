package admin

// unpricedflavors.go — the silent-zero-billing guard. The pricing engine bills a resource
// that matches NO rule as zero, without any error — so a (GPU) flavor shipped before its
// price rule exists runs free. GET /admin/service/{id}/unpriced-flavors lists the live
// flavors whose synthetic instance resource matches no rule of the service's enabled
// PUBLIC plans on any time unit. Profile-SCOPED plans are ignored: the guard targets the
// public default rate card. ADMIN_SERVICE_READ; cloud errors skip that region.

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/billingresource"
	"github.com/menlocloud/stratos/internal/platform/pricing"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// UnpricedFlavor is one live flavor with no matching price rule.
type UnpricedFlavor struct {
	Region   string `json:"region"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	GpuModel string `json:"gpuModel,omitempty"`
	GpuCount int    `json:"gpuCount,omitempty"`
}

// pricePlanTimeUnits are the rule time units a plan can price on.
var pricePlanTimeUnits = []string{"hour", "month", "minute"}

func (h *Handler) unpricedFlavors(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, "admin:service:read") {
		return
	}
	es, ok := h.loadServiceOr(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	ctx := r.Context()
	plans := pricing.SelectPricePlansForService(h.pricing.PlanSource(ctx), nil, true, es.ID)
	rules := []pricing.PricePlanRule{}
	for _, tu := range pricePlanTimeUnits {
		rules = append(rules, pricing.ApplicableRules(plans, h.pricing.RuleSource(ctx), tu)...)
	}
	var instanceType *pricing.BillingResourceType
	for _, t := range billingresource.Catalog() {
		if t.ResourceType == "instance" {
			instanceType = t
			break
		}
	}
	engine := pricing.NewEngine(nil)
	out := []UnpricedFlavor{}
	for _, region := range h.serviceRegions(es) {
		cc, err := h.cloudClient(ctx, es, region)
		if err != nil || cc == nil {
			continue
		}
		fs, err := cc.ListFlavors(ctx)
		if err != nil {
			continue
		}
		for _, f := range fs {
			model, count := cloud.GPUFromSpecStrings(f.ExtraSpecs)
			res := &pricing.BillingResource{
				ResourceType: "instance",
				Values: map[string]any{
					"instance_type": f.Name, "vcpus": f.VCPUs, "ram_mb": f.RAM,
					"ram_gb":       decimal.NewFromInt(int64(f.RAM)).DivRound(decimal.NewFromInt(1024), 2),
					"root_disk_gb": f.Disk, "gpu_model": model, "gpu_count": count,
					"is_bareMetal": false,
				},
				BillingResourceType: instanceType,
			}
			priced := false
			for _, tu := range pricePlanTimeUnits {
				if rs, err := engine.ApplyPricePlanRules(rules, res, tu); err == nil && len(rs) > 0 {
					priced = true
					break
				}
			}
			if !priced {
				out = append(out, UnpricedFlavor{Region: region, ID: f.ID, Name: f.Name, GpuModel: model, GpuCount: count})
			}
		}
	}
	httpx.List(w, out)
}
