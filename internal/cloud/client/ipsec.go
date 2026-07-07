package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ikepolicies"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/ipsecpolicies"
	"github.com/gophercloud/gophercloud/v2/openstack/networking/v2/extensions/vpnaas/siteconnections"
)

// ipsec.go = the Neutron VPNaaS read/write surface on the CloudClient facade. VPNaaS lives under the
// networking endpoint, so each call
// builds the network V2 client. Three CloudResource types share this file:
//   - IKE_POLICY            externalId == ikePolicy id     data = DataIKEPolicy{ikePolicy}
//   - IPSEC_POLICY          externalId == ipSecPolicy id   data = DataIPSecPolicy{ipSecPolicy}
//   - IPSEC_SITE_CONNECTION externalId == connection id    data = DataIPSecSiteConnection{ipSecSiteConnection}
// Each call returns the object as a free-form map[string]any (the CloudResource.data shape), so the
// SDK type never leaks. Gating = the networking "vpnaas" extension must be enabled. The
// networking V2 service client is built via c.vpn() (defined in vpn.go — VPNaaS shares the endpoint).

// redact drops write-only secrets (IPSec PSK, security-service password) from a free-form cloud
// object before it is cached in cr.Data or returned to the client. The create/get responses echo
// these back, but they must never persist or reach the FE. delete on a nil map is a no-op.
func redact(m map[string]any, keys ...string) map[string]any {
	for _, k := range keys {
		delete(m, k)
	}
	return m
}

// ---------------------------------------------------------------------------
// IKE policy (ikepolicies)
// ---------------------------------------------------------------------------

// CreateIKEPolicyOpts mirrors CreateIKEPolicyRequest. Neutron defaults any omitted field.
type CreateIKEPolicyOpts struct {
	Name                  string
	Description           string
	AuthAlgorithm         string
	EncryptionAlgorithm   string
	PFS                   string
	Phase1NegotiationMode string
	IKEVersion            string
}

// CreateIKEPolicy creates a Neutron VPN IKE policy.
func (c *Client) CreateIKEPolicy(ctx context.Context, o CreateIKEPolicyOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	p, err := ikepolicies.Create(ctx, nc, ikepolicies.CreateOpts{
		Name:                  o.Name,
		Description:           o.Description,
		AuthAlgorithm:         ikepolicies.AuthAlgorithm(o.AuthAlgorithm),
		EncryptionAlgorithm:   ikepolicies.EncryptionAlgorithm(o.EncryptionAlgorithm),
		PFS:                   ikepolicies.PFS(o.PFS),
		Phase1NegotiationMode: ikepolicies.Phase1NegotiationMode(o.Phase1NegotiationMode),
		IKEVersion:            ikepolicies.IKEVersion(o.IKEVersion),
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(p), nil
}

// GetIKEPolicy fetches a Neutron VPN IKE policy.
func (c *Client) GetIKEPolicy(ctx context.Context, id string) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	p, err := ikepolicies.Get(ctx, nc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(p), nil
}

// ListIKEPolicies lists the project's Neutron VPN IKE policies.
func (c *Client) ListIKEPolicies(ctx context.Context) ([]map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	pages, err := ikepolicies.List(nc, ikepolicies.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := ikepolicies.ExtractPolicies(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ps))
	for i := range ps {
		out = append(out, toMap(ps[i]))
	}
	return out, nil
}

// UpdateIKEPolicyOpts mirrors the IKE_POLICY UPDATE action.
type UpdateIKEPolicyOpts struct {
	Name                  string
	Description           string
	AuthAlgorithm         string
	EncryptionAlgorithm   string
	PFS                   string
	Phase1NegotiationMode string
	IKEVersion            string
}

// UpdateIKEPolicy updates a Neutron VPN IKE policy (IKE_POLICY action UPDATE).
func (c *Client) UpdateIKEPolicy(ctx context.Context, id string, o UpdateIKEPolicyOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	opts := ikepolicies.UpdateOpts{
		AuthAlgorithm:         ikepolicies.AuthAlgorithm(o.AuthAlgorithm),
		EncryptionAlgorithm:   ikepolicies.EncryptionAlgorithm(o.EncryptionAlgorithm),
		PFS:                   ikepolicies.PFS(o.PFS),
		Phase1NegotiationMode: ikepolicies.Phase1NegotiationMode(o.Phase1NegotiationMode),
		IKEVersion:            ikepolicies.IKEVersion(o.IKEVersion),
	}
	if o.Name != "" {
		opts.Name = &o.Name
	}
	if o.Description != "" {
		opts.Description = &o.Description
	}
	p, err := ikepolicies.Update(ctx, nc, id, opts).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(p), nil
}

// DeleteIKEPolicy removes a Neutron VPN IKE policy.
func (c *Client) DeleteIKEPolicy(ctx context.Context, id string) error {
	nc, err := c.vpn()
	if err != nil {
		return err
	}
	return ikepolicies.Delete(ctx, nc, id).ExtractErr()
}

// ---------------------------------------------------------------------------
// IPSec policy (ipsecpolicies)
// ---------------------------------------------------------------------------

// CreateIPSecPolicyOpts mirrors CreateIPSecPolicyRequest. Neutron defaults any omitted field.
type CreateIPSecPolicyOpts struct {
	Name                string
	Description         string
	EncryptionAlgorithm string
	PFS                 string
	TransformProtocol   string
	EncapsulationMode   string
	AuthAlgorithm       string
}

// CreateIPSecPolicy creates a Neutron VPN IPSec policy.
func (c *Client) CreateIPSecPolicy(ctx context.Context, o CreateIPSecPolicyOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	p, err := ipsecpolicies.Create(ctx, nc, ipsecpolicies.CreateOpts{
		Name:                o.Name,
		Description:         o.Description,
		EncryptionAlgorithm: ipsecpolicies.EncryptionAlgorithm(o.EncryptionAlgorithm),
		PFS:                 ipsecpolicies.PFS(o.PFS),
		TransformProtocol:   ipsecpolicies.TransformProtocol(o.TransformProtocol),
		EncapsulationMode:   ipsecpolicies.EncapsulationMode(o.EncapsulationMode),
		AuthAlgorithm:       ipsecpolicies.AuthAlgorithm(o.AuthAlgorithm),
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(p), nil
}

// GetIPSecPolicy fetches a Neutron VPN IPSec policy.
func (c *Client) GetIPSecPolicy(ctx context.Context, id string) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	p, err := ipsecpolicies.Get(ctx, nc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(p), nil
}

// ListIPSecPolicies lists the project's Neutron VPN IPSec policies.
func (c *Client) ListIPSecPolicies(ctx context.Context) ([]map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	pages, err := ipsecpolicies.List(nc, ipsecpolicies.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := ipsecpolicies.ExtractPolicies(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ps))
	for i := range ps {
		out = append(out, toMap(ps[i]))
	}
	return out, nil
}

// UpdateIPSecPolicyOpts mirrors the IPSEC_POLICY UPDATE action.
type UpdateIPSecPolicyOpts struct {
	Name                string
	Description         string
	EncryptionAlgorithm string
	PFS                 string
	TransformProtocol   string
	EncapsulationMode   string
	AuthAlgorithm       string
}

// UpdateIPSecPolicy updates a Neutron VPN IPSec policy (IPSEC_POLICY action UPDATE).
func (c *Client) UpdateIPSecPolicy(ctx context.Context, id string, o UpdateIPSecPolicyOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	opts := ipsecpolicies.UpdateOpts{
		EncryptionAlgorithm: ipsecpolicies.EncryptionAlgorithm(o.EncryptionAlgorithm),
		PFS:                 ipsecpolicies.PFS(o.PFS),
		TransformProtocol:   ipsecpolicies.TransformProtocol(o.TransformProtocol),
		EncapsulationMode:   ipsecpolicies.EncapsulationMode(o.EncapsulationMode),
		AuthAlgorithm:       ipsecpolicies.AuthAlgorithm(o.AuthAlgorithm),
	}
	if o.Name != "" {
		opts.Name = &o.Name
	}
	if o.Description != "" {
		opts.Description = &o.Description
	}
	p, err := ipsecpolicies.Update(ctx, nc, id, opts).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(p), nil
}

// DeleteIPSecPolicy removes a Neutron VPN IPSec policy.
func (c *Client) DeleteIPSecPolicy(ctx context.Context, id string) error {
	nc, err := c.vpn()
	if err != nil {
		return err
	}
	return ipsecpolicies.Delete(ctx, nc, id).ExtractErr()
}

// ---------------------------------------------------------------------------
// IPSec site connection (siteconnections)
// ---------------------------------------------------------------------------

// CreateIPSecSiteConnectionOpts mirrors CreateIPSecSiteConnectionRequest. peerId defaults to
// peerAddress (the request has no peerId field), so PeerID defaults to PeerAddress when blank.
type CreateIPSecSiteConnectionOpts struct {
	Name               string
	Description        string
	PeerAddress        string
	PeerID             string
	PSK                string
	Initiator          string
	VPNServiceID       string
	IKEPolicyID        string
	IPSecPolicyID      string
	LocalEndpointGroup string
	PeerEndpointGroup  string
	MTU                int
}

// CreateIPSecSiteConnection creates a Neutron VPN IPSec site connection.
func (c *Client) CreateIPSecSiteConnection(ctx context.Context, o CreateIPSecSiteConnectionOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	peerID := o.PeerID
	if peerID == "" {
		peerID = o.PeerAddress // peerId defaults to peerAddress
	}
	s, err := siteconnections.Create(ctx, nc, siteconnections.CreateOpts{
		Name:           o.Name,
		Description:    o.Description,
		PeerAddress:    o.PeerAddress,
		PeerID:         peerID,
		PSK:            o.PSK,
		Initiator:      siteconnections.Initiator(o.Initiator),
		VPNServiceID:   o.VPNServiceID,
		IKEPolicyID:    o.IKEPolicyID,
		IPSecPolicyID:  o.IPSecPolicyID,
		LocalEPGroupID: o.LocalEndpointGroup,
		PeerEPGroupID:  o.PeerEndpointGroup,
		MTU:            o.MTU,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return redact(toMap(s), "psk"), nil
}

// GetIPSecSiteConnection fetches a Neutron VPN IPSec site connection.
func (c *Client) GetIPSecSiteConnection(ctx context.Context, id string) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	s, err := siteconnections.Get(ctx, nc, id).Extract()
	if err != nil {
		return nil, err
	}
	return redact(toMap(s), "psk"), nil
}

// ListIPSecSiteConnections lists the project's Neutron VPN IPSec site connections.
func (c *Client) ListIPSecSiteConnections(ctx context.Context) ([]map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	pages, err := siteconnections.List(nc, siteconnections.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := siteconnections.ExtractConnections(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cs))
	for i := range cs {
		out = append(out, redact(toMap(cs[i]), "psk"))
	}
	return out, nil
}

// UpdateIPSecSiteConnectionOpts mirrors the IPSEC_SITE_CONNECTION UPDATE action.
type UpdateIPSecSiteConnectionOpts struct {
	Name        string
	Description string
	PeerAddress string
	PeerID      string
	PSK         string
	Initiator   string
	MTU         int
}

// UpdateIPSecSiteConnection updates a Neutron VPN IPSec site connection (IPSEC_SITE_CONNECTION
// action UPDATE).
func (c *Client) UpdateIPSecSiteConnection(ctx context.Context, id string, o UpdateIPSecSiteConnectionOpts) (map[string]any, error) {
	nc, err := c.vpn()
	if err != nil {
		return nil, err
	}
	opts := siteconnections.UpdateOpts{
		PeerAddress: o.PeerAddress,
		PeerID:      o.PeerID,
		PSK:         o.PSK,
		Initiator:   siteconnections.Initiator(o.Initiator),
		MTU:         o.MTU,
	}
	if o.Name != "" {
		opts.Name = &o.Name
	}
	if o.Description != "" {
		opts.Description = &o.Description
	}
	s, err := siteconnections.Update(ctx, nc, id, opts).Extract()
	if err != nil {
		return nil, err
	}
	return redact(toMap(s), "psk"), nil
}

// DeleteIPSecSiteConnection removes a Neutron VPN IPSec site connection.
func (c *Client) DeleteIPSecSiteConnection(ctx context.Context, id string) error {
	nc, err := c.vpn()
	if err != nil {
		return err
	}
	return siteconnections.Delete(ctx, nc, id).ExtractErr()
}
