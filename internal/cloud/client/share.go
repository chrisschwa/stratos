package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/shares"
)

// share.go = the Manila (shared-file-system) share read/write surface on the CloudClient facade. A share
// is the SHARE CloudResource; externalId == the
// Manila share id. data = DataShare{share}. Manila requires a microversion header (2.0 minimum).

func (c *Client) share() (*gophercloud.ServiceClient, error) {
	sc, err := openstack.NewSharedFileSystemV2(c.provider, c.endpointOpts())
	if err != nil {
		return nil, err
	}
	sc.Microversion = "2.0"
	return sc, nil
}

// CreateShareOpts mirrors CreateShareRequest: name + protocol +
// size (GB) are the required trio; shareType/shareNetworkId/availabilityZone are backend-dependent.
type CreateShareOpts struct {
	Name             string
	Description      string
	Protocol         string
	Size             int
	ShareType        string
	ShareNetworkID   string
	ShareGroupID     string
	AvailabilityZone string
}

// CreateShare creates a Manila share (provisions creating → available async).
func (c *Client) CreateShare(ctx context.Context, o CreateShareOpts) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	s, err := shares.Create(ctx, sc, shares.CreateOpts{
		Name: o.Name, Description: o.Description, ShareProto: o.Protocol, Size: o.Size,
		ShareType: o.ShareType, ShareNetworkID: o.ShareNetworkID, ShareGroupID: o.ShareGroupID,
		AvailabilityZone: o.AvailabilityZone,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(s), nil
}

// GetShare fetches a Manila share.
func (c *Client) GetShare(ctx context.Context, id string) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	s, err := shares.Get(ctx, sc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(s), nil
}

// DeleteShare removes a Manila share.
func (c *Client) DeleteShare(ctx context.Context, id string) error {
	sc, err := c.share()
	if err != nil {
		return err
	}
	return shares.Delete(ctx, sc, id).ExtractErr()
}
