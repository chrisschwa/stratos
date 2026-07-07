package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// PortProvider lists Neutron ports → CloudResource.
// `data.port.networkId` is the key the metrics classifier reads
// (metrics.isPublicTraffic → portNetworkID), so the cache must carry it. deviceId/owner/mac
// are kept for a future port billing provider.
type PortProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewPortProvider(cc *client.Client, region, projectID string) *PortProvider {
	return &PortProvider{cc: cc, region: region, projectID: projectID}
}

func (p *PortProvider) Type() string      { return cloud.TypePort }
func (p *PortProvider) ProjectID() string { return p.projectID }

func (p *PortProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	ports, err := p.cc.ListPorts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]cloud.CloudResource, 0, len(ports))
	for _, pt := range ports {
		out = append(out, cloud.CloudResource{
			Type:       cloud.TypePort,
			ExternalID: pt.ID,
			Region:     p.region,
			ProjectID:  p.projectID,
			Data: map[string]any{
				"port": map[string]any{
					"networkId":   pt.NetworkID,
					"deviceId":    pt.DeviceID,
					"deviceOwner": pt.DeviceOwner,
					"name":        pt.Name,
					"macAddress":  pt.MACAddress,
					"status":      pt.Status,
				},
			},
		})
	}
	return out, nil
}
