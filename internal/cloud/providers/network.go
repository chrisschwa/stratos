package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// NetworkProvider lists Neutron networks → CloudResource.
// `data.network` holds the network payload (the exact DataNetwork shape — NeutronNetwork +
// clusterInfo — is refined when golden-verifying against a live cloud; the rating path keys off
// externalId/type, not the data internals).
type NetworkProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewNetworkProvider(cc *client.Client, region, projectID string) *NetworkProvider {
	return &NetworkProvider{cc: cc, region: region, projectID: projectID}
}

func (p *NetworkProvider) Type() string { return cloud.TypeNetwork }

func (p *NetworkProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	nets, err := p.cc.ListNetworks(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]cloud.CloudResource, 0, len(nets))
	for _, n := range nets {
		out = append(out, cloud.CloudResource{
			Type:       cloud.TypeNetwork,
			ExternalID: n.ID,
			Region:     p.region,
			ProjectID:  p.projectID,
			Data:       map[string]any{"network": map[string]any{"id": n.ID, "name": n.Name}},
		})
	}
	return out, nil
}
