package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/baremetal/v1/nodes"
)

// baremetal.go = the Ironic (bare-metal) node read/write surface on the CloudClient facade. A bare-metal
// node is the BAREMETAL_SERVER CloudResource; its externalId == the Ironic node UUID. Each call returns
// the object as a free-form map[string]any (the CloudResource.data shape) so the SDK type never leaks.
//
// NOTE: this surface also has a nova-server-backed create + the GET_FLAVOR/LIST_FLAVORS
// actions; those reuse the existing compute helpers (CreateServer / GetFlavor / ListFlavors in
// client.go) — this file is the Ironic-node-specific surface (provision/power lifecycle).

// baremetal builds the Ironic v1 service client. Ironic requires an API microversion to expose the
// modern fields (owner, conductor, etc.); "1.46" is a safe floor (owner + maintenance + power
// states are all available).
func (c *Client) baremetal() (*gophercloud.ServiceClient, error) {
	bc, err := openstack.NewBareMetalV1(c.provider, c.endpointOpts())
	if err != nil {
		return nil, err
	}
	bc.Microversion = "1.46"
	return bc, nil
}

// CreateBareMetalNodeOpts mirrors the Ironic node create body (node enroll). driver is
// the only hard requirement; the rest are optional per-driver fields.
type CreateBareMetalNodeOpts struct {
	Name           string
	Driver         string
	ResourceClass  string
	DriverInfo     map[string]any
	Properties     map[string]any
	Extra          map[string]any
	ConductorGroup string
	Owner          string
}

// CreateBareMetalNode enrolls a new Ironic node (provision_state = "enroll").
func (c *Client) CreateBareMetalNode(ctx context.Context, o CreateBareMetalNodeOpts) (map[string]any, error) {
	bc, err := c.baremetal()
	if err != nil {
		return nil, err
	}
	n, err := nodes.Create(ctx, bc, nodes.CreateOpts{
		Name:           o.Name,
		Driver:         o.Driver,
		ResourceClass:  o.ResourceClass,
		DriverInfo:     o.DriverInfo,
		Properties:     o.Properties,
		Extra:          o.Extra,
		ConductorGroup: o.ConductorGroup,
		Owner:          o.Owner,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(n), nil
}

// GetBareMetalNode fetches an Ironic node by UUID (or name).
func (c *Client) GetBareMetalNode(ctx context.Context, id string) (map[string]any, error) {
	bc, err := c.baremetal()
	if err != nil {
		return nil, err
	}
	n, err := nodes.Get(ctx, bc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(n), nil
}

// ListBareMetalNodes lists the project's Ironic nodes (detailed).
func (c *Client) ListBareMetalNodes(ctx context.Context) ([]map[string]any, error) {
	bc, err := c.baremetal()
	if err != nil {
		return nil, err
	}
	pages, err := nodes.ListDetail(bc, nodes.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ns, err := nodes.ExtractNodes(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ns))
	for i := range ns {
		out = append(out, toMap(ns[i]))
	}
	return out, nil
}

// DeleteBareMetalNode removes an Ironic node by UUID.
func (c *Client) DeleteBareMetalNode(ctx context.Context, id string) error {
	bc, err := c.baremetal()
	if err != nil {
		return err
	}
	return nodes.Delete(ctx, bc, id).ExtractErr()
}

// SetBareMetalNodePower changes the node's power state (power on/off action). target =
// "power on" / "power off" / "rebooting" / "soft power off" / "soft rebooting". Async (202).
func (c *Client) SetBareMetalNodePower(ctx context.Context, id, target string) error {
	bc, err := c.baremetal()
	if err != nil {
		return err
	}
	return nodes.ChangePowerState(ctx, bc, id, nodes.PowerStateOpts{
		Target: nodes.TargetPowerState(target),
	}).ExtractErr()
}

// SetBareMetalNodeProvisionState drives the node through the Ironic provision state machine
// (manage/provide/deploy/undeploy actions). target = "active" / "deleted" /
// "manage" / "provide" / "inspect" / "clean" / "rebuild" / etc. Async (202).
func (c *Client) SetBareMetalNodeProvisionState(ctx context.Context, id, target string) error {
	bc, err := c.baremetal()
	if err != nil {
		return err
	}
	return nodes.ChangeProvisionState(ctx, bc, id, nodes.ProvisionStateOpts{
		Target: nodes.TargetProvisionState(target),
	}).ExtractErr()
}

// SetBareMetalNodeMaintenance toggles the node's maintenance mode (maintenance
// action). on=true sets it (with an optional reason), on=false unsets it. Async (202).
func (c *Client) SetBareMetalNodeMaintenance(ctx context.Context, id string, on bool, reason string) error {
	bc, err := c.baremetal()
	if err != nil {
		return err
	}
	if on {
		return nodes.SetMaintenance(ctx, bc, id, nodes.MaintenanceOpts{Reason: reason}).ExtractErr()
	}
	return nodes.UnsetMaintenance(ctx, bc, id).ExtractErr()
}

// ValidateBareMetalNode polls each driver interface and returns the per-interface validation status
// (validate action).
func (c *Client) ValidateBareMetalNode(ctx context.Context, id string) (map[string]any, error) {
	bc, err := c.baremetal()
	if err != nil {
		return nil, err
	}
	v, err := nodes.Validate(ctx, bc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(v), nil
}
