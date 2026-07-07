package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/loadbalancers"
)

// loadbalancer.go = the Octavia (load-balancer) read/write surface on the CloudClient facade. A load
// balancer is the LOAD_BALANCER
// CloudResource; its externalId == the Octavia LB id. data = DataLoadBalancer{loadBalancer}.

func (c *Client) loadbalancer() (*gophercloud.ServiceClient, error) {
	return openstack.NewLoadBalancerV2(c.provider, c.endpointOpts())
}

// CreateLoadBalancerOpts mirrors the base CreateLoadBalancerRequest (name + networkId=networkExternalId,
// optional AZ). The
// listener/pool/member/monitor sub-resources are managed by separate actions (not the base create).
type CreateLoadBalancerOpts struct {
	Name             string
	NetworkID        string // → vip_network_id (the network externalId)
	AvailabilityZone string
}

// CreateLoadBalancer creates an Octavia load balancer (provisions PENDING_CREATE → ACTIVE async).
func (c *Client) CreateLoadBalancer(ctx context.Context, o CreateLoadBalancerOpts) (map[string]any, error) {
	lc, err := c.loadbalancer()
	if err != nil {
		return nil, err
	}
	lb, err := loadbalancers.Create(ctx, lc, loadbalancers.CreateOpts{
		Name: o.Name, VipNetworkID: o.NetworkID, AvailabilityZone: o.AvailabilityZone,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(lb), nil
}

// GetLoadBalancer fetches an Octavia load balancer.
func (c *Client) GetLoadBalancer(ctx context.Context, id string) (map[string]any, error) {
	lc, err := c.loadbalancer()
	if err != nil {
		return nil, err
	}
	lb, err := loadbalancers.Get(ctx, lc, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(lb), nil
}

// (a live list of LOAD_BALANCER resources is served by the typed ListLoadBalancers in list_extra.go,
// used by the sync job; the create/cache path needs only Create/Get/Delete here.)

// DeleteLoadBalancer cascade-deletes an Octavia load balancer + its children (cascade delete).
func (c *Client) DeleteLoadBalancer(ctx context.Context, id string) error {
	lc, err := c.loadbalancer()
	if err != nil {
		return err
	}
	return loadbalancers.Delete(ctx, lc, id, loadbalancers.DeleteOpts{Cascade: true}).ExtractErr()
}
