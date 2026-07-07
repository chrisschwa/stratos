package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/endpointgroups"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/services"
)

// vpn.go = the Neutron VPNaaS read/write surface on the CloudClient facade (VPN services and VPN
// endpoint groups). VPNaaS lives under the networking (Neutron) service, so the service
// client is built with openstack.NewNetworkV2. Two CloudResources:
//   - VPN_SERVICE        externalId == the vpnservice id   (data = DataVPNService{vpnService})
//   - VPN_ENDPOINT_GROUP externalId == the endpoint_group id (data = DataVPNEndpointGroup{vpnEndpointGroup})
// Each call returns the object as a free-form map[string]any (the CloudResource.data shape), so the
// SDK type never leaks. NOTE: this region has no VPNaaS backend (live-blocked) — these are real
// gophercloud calls that only succeed where the vpnaas extension is enabled.

func (c *Client) vpn() (*gophercloud.ServiceClient, error) {
	return openstack.NewNetworkV2(c.provider, c.endpointOpts())
}

// --- VPN service (vpnservice) ---------------------------------------------------------------------

// CreateVPNServiceOpts mirrors the base CreateVPNServiceRequest: name/description/adminStateUp + the
// routerId (required) / subnetId / flavorId. RouterID maps from the FE externalRouterId, SubnetID
// from externalSubnetId.
type CreateVPNServiceOpts struct {
	Name         string
	Description  string
	AdminStateUp bool
	RouterID     string // → router_id (the external router id, required by Neutron)
	SubnetID     string // → subnet_id (the external subnet id)
	FlavorID     string
}

// CreateVPNService creates a Neutron VPN service (provisions PENDING_CREATE → ACTIVE async).
func (c *Client) CreateVPNService(ctx context.Context, o CreateVPNServiceOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	adminUp := o.AdminStateUp
	s, err := services.Create(ctx, nc, services.CreateOpts{
		Name:         o.Name,
		Description:  o.Description,
		AdminStateUp: &adminUp,
		RouterID:     o.RouterID,
		SubnetID:     o.SubnetID,
		FlavorID:     o.FlavorID,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(s), nil
}

// GetVPNService fetches a Neutron VPN service.
func (c *Client) GetVPNService(ctx context.Context, id string) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	s, err := services.Get(ctx, nc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(s), nil
}

// ListVPNServices lists the project's Neutron VPN services. Scoped by the
// client's tenant (empty projectID → unfiltered admin probe, mirroring the neutron list pattern).
func (c *Client) ListVPNServices(ctx context.Context) ([]map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	pages, err := services.List(nc, services.ListOpts{ProjectID: c.projectID}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ss, err := services.ExtractServices(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ss))
	for i := range ss {
		out = append(out, toMap(ss[i]))
	}
	return out, nil
}

// DeleteVPNService removes a Neutron VPN service.
func (c *Client) DeleteVPNService(ctx context.Context, id string) error {
	nc, err := c.vpn()
	if err != nil {
		return err
	}
	return services.Delete(ctx, nc, id).ExtractErr()
}

// --- VPN endpoint group (endpoint_group) ----------------------------------------------------------

// CreateVPNEndpointGroupOpts mirrors CreateVPNEndpointGroupRequest: name/description + the
// endpoint type (subnet/cidr/network/router/vlan) and the list of endpoints of that type.
type CreateVPNEndpointGroupOpts struct {
	Name        string
	Description string
	Type        string
	Endpoints   []string
}

// CreateVPNEndpointGroup creates a Neutron VPN endpoint group.
func (c *Client) CreateVPNEndpointGroup(ctx context.Context, o CreateVPNEndpointGroupOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	eg, err := endpointgroups.Create(ctx, nc, endpointgroups.CreateOpts{
		Name:        o.Name,
		Description: o.Description,
		Type:        endpointgroups.EndpointType(o.Type),
		Endpoints:   o.Endpoints,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(eg), nil
}

// GetVPNEndpointGroup fetches a Neutron VPN endpoint group.
func (c *Client) GetVPNEndpointGroup(ctx context.Context, id string) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	eg, err := endpointgroups.Get(ctx, nc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(eg), nil
}

// ListVPNEndpointGroups lists the project's Neutron VPN endpoint groups. Scoped by the client's
// tenant (empty projectID → unfiltered admin probe).
func (c *Client) ListVPNEndpointGroups(ctx context.Context) ([]map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	pages, err := endpointgroups.List(nc, endpointgroups.ListOpts{ProjectID: c.projectID}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	egs, err := endpointgroups.ExtractEndpointGroups(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(egs))
	for i := range egs {
		out = append(out, toMap(egs[i]))
	}
	return out, nil
}

// DeleteVPNEndpointGroup removes a Neutron VPN endpoint group.
func (c *Client) DeleteVPNEndpointGroup(ctx context.Context, id string) error {
	nc, err := c.vpn()
	if err != nil {
		return err
	}
	return endpointgroups.Delete(ctx, nc, id).ExtractErr()
}
