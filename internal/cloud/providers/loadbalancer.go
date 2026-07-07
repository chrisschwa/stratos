package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// LoadBalancerProvider lists Octavia load balancers → CloudResource.
type LoadBalancerProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewLoadBalancerProvider(cc *client.Client, region, projectID string) *LoadBalancerProvider {
	return &LoadBalancerProvider{cc: cc, region: region, projectID: projectID}
}

func (p *LoadBalancerProvider) Type() string      { return cloud.TypeLoadBalancer }
func (p *LoadBalancerProvider) ProjectID() string { return p.projectID }

func (p *LoadBalancerProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	lbs, err := p.cc.ListLoadBalancers(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]cloud.CloudResource, 0, len(lbs))
	for _, l := range lbs {
		out = append(out, cloud.CloudResource{
			Type:       cloud.TypeLoadBalancer,
			ExternalID: l.ID,
			Region:     p.region,
			ProjectID:  p.projectID,
			Data: map[string]any{"loadBalancer": map[string]any{
				"id": l.ID, "name": l.Name, "operating_status": l.OperatingStatus,
				"provisioning_status": l.ProvisioningStatus,
				"flavor_id":           l.FlavorID, "vip_network_id": l.VipNetworkID, "vip_address": l.VipAddress,
			}},
		})
	}
	return out, nil
}
