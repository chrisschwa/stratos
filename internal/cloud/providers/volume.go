package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// VolumeProvider lists Cinder volumes → CloudResource.
// `data.volume` holds the payload the volume BillingResource provider reads.
type VolumeProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewVolumeProvider(cc *client.Client, region, projectID string) *VolumeProvider {
	return &VolumeProvider{cc: cc, region: region, projectID: projectID}
}

func (p *VolumeProvider) Type() string      { return cloud.TypeVolume }
func (p *VolumeProvider) ProjectID() string { return p.projectID }

func (p *VolumeProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	vols, err := p.cc.ListVolumes(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]cloud.CloudResource, 0, len(vols))
	for _, v := range vols {
		out = append(out, cloud.CloudResource{
			Type:       cloud.TypeVolume,
			ExternalID: v.ID,
			Region:     p.region,
			ProjectID:  p.projectID,
			Data: map[string]any{"volume": map[string]any{
				"id": v.ID, "name": v.Name, "size": v.Size, "status": v.Status,
				"volume_type": v.VolumeType, "availability_zone": v.AvailabilityZone, "bootable": v.Bootable,
			}},
		})
	}
	return out, nil
}
