package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/recordsets"
	"github.com/gophercloud/gophercloud/v2/openstack/dns/v2/zones"
)

// dns.go = the Designate (DNS) write/read surface on the CloudClient facade. Zones are
// the DNS_ZONE CloudResource; recordsets are managed as actions on a zone. Each call returns the
// object as a free-form map[string]any (the CloudResource.data shape), so the SDK type never leaks.

func (c *Client) dns() (*gophercloud.ServiceClient, error) {
	return openstack.NewDNSV2(c.provider, c.endpointOpts())
}

// CreateZoneOpts mirrors the Designate zone create. Name must be the
// FQDN-with-trailing-dot (the provider normalizes the request's `domain` first).
type CreateZoneOpts struct {
	Name        string
	Email       string
	TTL         int
	Description string
}

// CreateZone creates a Designate zone.
func (c *Client) CreateZone(ctx context.Context, o CreateZoneOpts) (map[string]any, error) {
	dc, err := c.dns()
	if err != nil {
		return nil, err
	}
	z, err := zones.Create(ctx, dc, zones.CreateOpts{
		Name: o.Name, Email: o.Email, TTL: o.TTL, Description: o.Description,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(z), nil
}

// GetZone fetches a Designate zone.
func (c *Client) GetZone(ctx context.Context, id string) (map[string]any, error) {
	dc, err := c.dns()
	if err != nil {
		return nil, err
	}
	z, err := zones.Get(ctx, dc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(z), nil
}

// ListZones lists the project's Designate zones.
func (c *Client) ListZones(ctx context.Context) ([]map[string]any, error) {
	dc, err := c.dns()
	if err != nil {
		return nil, err
	}
	pages, err := zones.List(dc, zones.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	zs, err := zones.ExtractZones(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(zs))
	for i := range zs {
		out = append(out, toMap(zs[i]))
	}
	return out, nil
}

// DeleteZone removes a Designate zone.
func (c *Client) DeleteZone(ctx context.Context, id string) error {
	dc, err := c.dns()
	if err != nil {
		return err
	}
	_, err = zones.Delete(ctx, dc, id).Extract() // async delete returns the pending zone
	return err
}

// CreateRecordsetOpts mirrors DataDns.Recordset {name, type, ttl, records}.
type CreateRecordsetOpts struct {
	Name    string
	Type    string
	TTL     int
	Records []string
}

// CreateRecordset adds a recordset to a zone (ttl forced to 7200).
func (c *Client) CreateRecordset(ctx context.Context, zoneID string, o CreateRecordsetOpts) (map[string]any, error) {
	dc, err := c.dns()
	if err != nil {
		return nil, err
	}
	ttl := o.TTL
	if ttl == 0 {
		ttl = 7200 // default TTL
	}
	rs, err := recordsets.Create(ctx, dc, zoneID, recordsets.CreateOpts{
		Name: o.Name, Type: o.Type, TTL: ttl, Records: o.Records,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(rs), nil
}

// ListRecordsets lists a zone's recordsets.
func (c *Client) ListRecordsets(ctx context.Context, zoneID string) ([]map[string]any, error) {
	dc, err := c.dns()
	if err != nil {
		return nil, err
	}
	pages, err := recordsets.ListByZone(dc, zoneID, recordsets.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := recordsets.ExtractRecordSets(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rs))
	for i := range rs {
		out = append(out, toMap(rs[i]))
	}
	return out, nil
}

// DeleteRecordset removes a recordset from a zone.
func (c *Client) DeleteRecordset(ctx context.Context, zoneID, recordsetID string) error {
	dc, err := c.dns()
	if err != nil {
		return err
	}
	return recordsets.Delete(ctx, dc, zoneID, recordsetID).ExtractErr()
}

// UpdateRecordset updates a recordset's ttl/records.
func (c *Client) UpdateRecordset(ctx context.Context, zoneID, recordsetID string, ttl int, records []string) (map[string]any, error) {
	dc, err := c.dns()
	if err != nil {
		return nil, err
	}
	opts := recordsets.UpdateOpts{Records: records}
	if ttl > 0 {
		opts.TTL = &ttl
	}
	rs, err := recordsets.Update(ctx, dc, zoneID, recordsetID, opts).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(rs), nil
}
