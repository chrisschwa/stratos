package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// FloatingIPProvider lists Neutron floating IPs → CloudResource.
type FloatingIPProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewFloatingIPProvider(cc *client.Client, region, projectID string) *FloatingIPProvider {
	return &FloatingIPProvider{cc: cc, region: region, projectID: projectID}
}

func (p *FloatingIPProvider) Type() string      { return cloud.TypeFloatingIP }
func (p *FloatingIPProvider) ProjectID() string { return p.projectID }

func (p *FloatingIPProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	fips, err := p.cc.ListFloatingIPs(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]cloud.CloudResource, 0, len(fips))
	for _, f := range fips {
		out = append(out, cloud.CloudResource{
			Type:       cloud.TypeFloatingIP,
			ExternalID: f.ID,
			Region:     p.region,
			ProjectID:  p.projectID,
			Data: map[string]any{"floatingIp": map[string]any{
				"id": f.ID, "status": f.Status, "floating_ip_address": f.FloatingIP,
				"floating_network_id": f.FloatingNetworkID, "port_id": f.PortID,
			}},
		})
	}
	return out, nil
}
