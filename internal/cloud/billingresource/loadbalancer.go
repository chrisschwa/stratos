package billingresource

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/platform/pricing"
)

// LoadBalancerProvider maps a
// LOAD_BALANCER → a "load_balancer" BillingResource (flavor/status attributes).
type LoadBalancerProvider struct{}

func NewLoadBalancerProvider() *LoadBalancerProvider { return &LoadBalancerProvider{} }

func (p *LoadBalancerProvider) Type() string { return cloud.TypeLoadBalancer }

func (p *LoadBalancerProvider) GetBillingInformation(_ context.Context, _ pricing.BillingContext, cr *cloud.CloudResource) ([]*pricing.BillingResource, error) {
	values := map[string]any{}
	notEligible := false
	if lb, ok := mapAt(cr.Data, "loadBalancer"); ok {
		values["flavor_id"] = lb["flavor_id"]
		values["display_name"] = lb["name"]
		values["operating_status"] = lb["operating_status"]
		// operating_status == ERROR → not eligible.
		notEligible = str(lb["operating_status"]) == "ERROR"
	}
	return []*pricing.BillingResource{{
		ResourceID:            cr.ID,
		ProjectID:             cr.ProjectID,
		ResourceType:          "load_balancer",
		Values:                values,
		BillingResourceType:   loadBalancerType(),
		NotEligibleForBilling: notEligible,
	}}, nil
}

func loadBalancerType() *pricing.BillingResourceType {
	s := func(n string) pricing.ResourceAttribute { return pricing.ResourceAttribute{Name: n, Type: "string"} }
	return &pricing.BillingResourceType{ResourceType: "load_balancer", Attributes: []pricing.ResourceAttribute{
		s("flavor_id"), s("display_name"), s("operating_status"),
	}}
}
