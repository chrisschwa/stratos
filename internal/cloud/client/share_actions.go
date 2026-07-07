package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/securityservices"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/sharenetworks"
	"github.com/gophercloud/gophercloud/v2/openstack/sharedfilesystems/v2/shares"
)

// share_actions.go = the rest of the Manila (shared-file-system) surface on the CloudClient facade,
// beyond the base share Create/Get/Delete already in share.go. Covers the SHARE actions
// (GRANT_ACCESS/REVOKE_ACCESS/LIST_ACCESS/
// EXTEND_SHARE/SHRINK_SHARE), plus the SHARE_NETWORK and SHARE_SECURITY_SERVICE CRUD +
// network actions (ADD/REMOVE_SECURITY_SERVICE, UPDATE) — all over gophercloud
// sharedfilesystems/v2/{shares,sharenetworks,securityservices}. Each returns a free-form
// map[string]any / []map[string]any so the SDK type never leaks.
//
// MICROVERSION NOTE: the base share() helper pins "2.0". The share-access actions
// (grant/revoke/list-access, extend, shrink) require Manila microversion >= 2.7, so those use a
// dedicated shareMV("2.7") client. Everything else (share-network/security-service CRUD, neutron
// share-network create/update) works at "2.0".

// shareMV builds a Manila client pinned to a specific microversion (the access-rule actions need
// >= 2.7; the base share() helper stays at "2.0").
func (c *Client) shareMV(version string) (*gophercloud.ServiceClient, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	sc.Microversion = version
	return sc, nil
}

// ===========================================================================================
// SHARE actions (on an existing share, keyed by externalId == the Manila share id)
// ===========================================================================================

// GrantShareAccessOpts mirrors ShareGrantAccessRequest (access rule: level, type, accessTo).
type GrantShareAccessOpts struct {
	AccessType  string // ip | cert | user
	AccessTo    string
	AccessLevel string // rw | ro
}

// GrantShareAccess grants an access rule to a share (Manila share action allow_access, MV >= 2.7).
func (c *Client) GrantShareAccess(ctx context.Context, shareID string, o GrantShareAccessOpts) (map[string]any, error) {
	sc, err := c.shareMV("2.7")
	if err != nil {
		return nil, err
	}
	ar, err := shares.GrantAccess(ctx, sc, shareID, shares.GrantAccessOpts{
		AccessType: o.AccessType, AccessTo: o.AccessTo, AccessLevel: o.AccessLevel,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(ar), nil
}

// RevokeShareAccess revokes an access rule from a share (the payload field
// `ruleId` → the access rule id; Manila share action deny_access, MV >= 2.7).
func (c *Client) RevokeShareAccess(ctx context.Context, shareID, ruleID string) error {
	sc, err := c.shareMV("2.7")
	if err != nil {
		return err
	}
	return shares.RevokeAccess(ctx, sc, shareID, shares.RevokeAccessOpts{AccessID: ruleID}).ExtractErr()
}

// ListShareAccess lists a share's access rules (Manila share action access_list, MV >= 2.7).
func (c *Client) ListShareAccess(ctx context.Context, shareID string) ([]map[string]any, error) {
	sc, err := c.shareMV("2.7")
	if err != nil {
		return nil, err
	}
	rights, err := shares.ListAccessRights(ctx, sc, shareID).Extract()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rights))
	for i := range rights {
		out = append(out, toMap(rights[i]))
	}
	return out, nil
}

// ExtendShare extends a share to a new size in GB (the payload field
// `size`; Manila share action extend, MV >= 2.7). Returns nothing (202).
func (c *Client) ExtendShare(ctx context.Context, shareID string, newSize int) error {
	sc, err := c.shareMV("2.7")
	if err != nil {
		return err
	}
	return shares.Extend(ctx, sc, shareID, shares.ExtendOpts{NewSize: newSize}).ExtractErr()
}

// ShrinkShare shrinks a share to a new size in GB (the payload field
// `accessId` carries the new size — a deliberate quirk; Manila share action shrink, MV >= 2.7).
func (c *Client) ShrinkShare(ctx context.Context, shareID string, newSize int) error {
	sc, err := c.shareMV("2.7")
	if err != nil {
		return err
	}
	return shares.Shrink(ctx, sc, shareID, shares.ShrinkOpts{NewSize: newSize}).ExtractErr()
}

// ===========================================================================================
// SHARE_NETWORK CRUD + actions
// ===========================================================================================

// CreateShareNetworkOpts mirrors CreateShareNetworkRequest
// (neutron_net_id ← externalNetworkId, neutron_subnet_id ← externalSubnetId).
type CreateShareNetworkOpts struct {
	Name              string
	Description       string
	ExternalNetworkID string // → neutron_net_id
	ExternalSubnetID  string // → neutron_subnet_id
}

// CreateShareNetwork creates a Manila share network bound to a neutron net/subnet.
func (c *Client) CreateShareNetwork(ctx context.Context, o CreateShareNetworkOpts) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	sn, err := sharenetworks.Create(ctx, sc, sharenetworks.CreateOpts{
		Name: o.Name, Description: o.Description,
		NeutronNetID: o.ExternalNetworkID, NeutronSubnetID: o.ExternalSubnetID,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(sn), nil
}

// GetShareNetwork fetches a Manila share network.
func (c *Client) GetShareNetwork(ctx context.Context, id string) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	sn, err := sharenetworks.Get(ctx, sc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(sn), nil
}

// ListShareNetworks lists the project's Manila share networks.
func (c *Client) ListShareNetworks(ctx context.Context) ([]map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	pages, err := sharenetworks.ListDetail(sc, sharenetworks.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	sns, err := sharenetworks.ExtractShareNetworks(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(sns))
	for i := range sns {
		out = append(out, toMap(sns[i]))
	}
	return out, nil
}

// DeleteShareNetwork removes a Manila share network.
func (c *Client) DeleteShareNetwork(ctx context.Context, id string) error {
	sc, err := c.share()
	if err != nil {
		return err
	}
	return sharenetworks.Delete(ctx, sc, id).ExtractErr()
}

// AddSecurityServiceToNetwork attaches a security service to a share network.
// Returns the updated share network.
func (c *Client) AddSecurityServiceToNetwork(ctx context.Context, shareNetworkID, securityServiceID string) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	sn, err := sharenetworks.AddSecurityService(ctx, sc, shareNetworkID,
		sharenetworks.AddSecurityServiceOpts{SecurityServiceID: securityServiceID}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(sn), nil
}

// RemoveSecurityServiceFromNetwork detaches a security service from a share network.
// Returns the updated share network.
func (c *Client) RemoveSecurityServiceFromNetwork(ctx context.Context, shareNetworkID, securityServiceID string) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	sn, err := sharenetworks.RemoveSecurityService(ctx, sc, shareNetworkID,
		sharenetworks.RemoveSecurityServiceOpts{SecurityServiceID: securityServiceID}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(sn), nil
}

// UpdateShareNetwork updates a share network's neutron net/subnet binding. The field mapping is
// deliberately SWAPPED:
// neutron_net_id ← payload `subnetId`, neutron_subnet_id ← payload `networkId`.
func (c *Client) UpdateShareNetwork(ctx context.Context, shareNetworkID, neutronNetID, neutronSubnetID string) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	sn, err := sharenetworks.Update(ctx, sc, shareNetworkID, sharenetworks.UpdateOpts{
		NeutronNetID: neutronNetID, NeutronSubnetID: neutronSubnetID,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(sn), nil
}

// ===========================================================================================
// SHARE_SECURITY_SERVICE CRUD
// ===========================================================================================

// CreateShareSecurityServiceOpts carries the create-share-security-service fields.
type CreateShareSecurityServiceOpts struct {
	Name        string
	Description string
	Type        string // ldap | kerberos | active_directory
	DNSIP       string
	User        string
	Password    string
	Domain      string
	Server      string
}

// CreateShareSecurityService creates a Manila security service.
func (c *Client) CreateShareSecurityService(ctx context.Context, o CreateShareSecurityServiceOpts) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	ss, err := securityservices.Create(ctx, sc, securityservices.CreateOpts{
		Type:        securityservices.SecurityServiceType(o.Type),
		Name:        o.Name,
		Description: o.Description,
		DNSIP:       o.DNSIP,
		User:        o.User,
		Password:    o.Password,
		Domain:      o.Domain,
		Server:      o.Server,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return redact(toMap(ss), "password"), nil
}

// GetShareSecurityService fetches a Manila security service.
func (c *Client) GetShareSecurityService(ctx context.Context, id string) (map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	ss, err := securityservices.Get(ctx, sc, id).Extract()
	if err != nil {
		return nil, err
	}
	return redact(toMap(ss), "password"), nil
}

// ListShareSecurityServices lists the project's Manila security services.
func (c *Client) ListShareSecurityServices(ctx context.Context) ([]map[string]any, error) {
	sc, err := c.share()
	if err != nil {
		return nil, err
	}
	pages, err := securityservices.List(sc, securityservices.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	sss, err := securityservices.ExtractSecurityServices(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(sss))
	for i := range sss {
		out = append(out, redact(toMap(sss[i]), "password"))
	}
	return out, nil
}

// DeleteShareSecurityService removes a Manila security service.
func (c *Client) DeleteShareSecurityService(ctx context.Context, id string) error {
	sc, err := c.share()
	if err != nil {
		return err
	}
	return securityservices.Delete(ctx, sc, id).ExtractErr()
}
