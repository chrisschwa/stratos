package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/listeners"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/monitors"
	"github.com/gophercloud/gophercloud/v2/openstack/loadbalancer/v2/pools"
)

// loadbalancer_sub.go = the Octavia listener/pool/member/health-monitor sub-resource surface on the
// CloudClient facade (the listener/pool/member/monitor methods driven by
// the DataLoadBalancer.Action enum: GET_LISTENERS/CREATE_LISTENER/DELETE_LISTENER, GET_POOLS/
// CREATE_POOL/DELETE_POOL, ADD_MEMBER/DELETE_MEMBER, GET_MONITORS/ADD_MONITOR/DELETE_MONITOR). The
// base LB create/get/delete live in loadbalancer.go; these manage the children. Each call returns a
// free-form map[string]any (single) or []map[string]any (list) so the gophercloud type never leaks.
//
// All sub-resources are reached via the same Octavia LoadBalancerV2 service client (reuses
// loadbalancer.go's c.loadbalancer()); the lower-level packages (listeners/pools/monitors) share
// that endpoint.

func (c *Client) lbSub() (*gophercloud.ServiceClient, error) {
	return openstack.NewLoadBalancerV2(c.provider, c.endpointOpts())
}

// ---- listeners (GET_LISTENERS / CREATE_LISTENER / DELETE_LISTENER) ----

// GetListeners lists the listeners of one load balancer (list filtered by
// loadbalancer_id). lbID is the Octavia LB externalId.
func (c *Client) GetListeners(ctx context.Context, lbID string) ([]map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	pages, err := listeners.List(lc, listeners.ListOpts{LoadbalancerID: lbID}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ls, err := listeners.ExtractListeners(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ls))
	for i := range ls {
		out = append(out, toMap(ls[i]))
	}
	return out, nil
}

// CreateListenerOpts mirrors DataLoadBalancer.LbListener (the CREATE_LISTENER action body). PoolID
// is optional (default_pool_id is set only when non-blank).
type CreateListenerOpts struct {
	Name            string
	Protocol        string
	ListenerPort    int
	LoadBalancerID  string // → loadbalancer_id (the cloud resource externalId)
	ConnectionLimit int
	PoolID          string
}

// CreateListener creates a listener on a load balancer.
func (c *Client) CreateListener(ctx context.Context, o CreateListenerOpts) (map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	opts := listeners.CreateOpts{
		Name:           o.Name,
		Protocol:       listeners.Protocol(o.Protocol),
		ProtocolPort:   o.ListenerPort,
		LoadbalancerID: o.LoadBalancerID,
	}
	if o.ConnectionLimit != 0 {
		cl := o.ConnectionLimit
		opts.ConnLimit = &cl
	}
	if o.PoolID != "" {
		opts.DefaultPoolID = o.PoolID
	}
	l, err := listeners.Create(ctx, lc, opts).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(l), nil
}

// DeleteListener removes a listener by id.
func (c *Client) DeleteListener(ctx context.Context, listenerID string) error {
	lc, err := c.lbSub()
	if err != nil {
		return err
	}
	return listeners.Delete(ctx, lc, listenerID).ExtractErr()
}

// ---- pools + members (GET_POOLS / CREATE_POOL / DELETE_POOL / ADD_MEMBER / DELETE_MEMBER) ----

// GetPools lists the pools of one load balancer, each enriched with its members
// (list filtered by loadbalancer_id, then members attached). lbID is the Octavia LB externalId.
func (c *Client) GetPools(ctx context.Context, lbID string) ([]map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	pages, err := pools.List(lc, pools.ListOpts{LoadbalancerID: lbID}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ps, err := pools.ExtractPools(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ps))
	for i := range ps {
		m := toMap(ps[i])
		// Re-list the pool's members and attach them.
		if ms, merr := c.listPoolMembers(ctx, lc, ps[i].ID); merr == nil {
			m["members"] = ms
		}
		out = append(out, m)
	}
	return out, nil
}

// listPoolMembers lists a pool's members as free-form maps (shared by GetPools / GetMembers).
func (c *Client) listPoolMembers(ctx context.Context, lc *gophercloud.ServiceClient, poolID string) ([]map[string]any, error) {
	pages, err := pools.ListMembers(lc, poolID, pools.ListMembersOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ms, err := pools.ExtractMembers(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ms))
	for i := range ms {
		out = append(out, toMap(ms[i]))
	}
	return out, nil
}

// GetMembers lists the members of a pool.
func (c *Client) GetMembers(ctx context.Context, poolID string) ([]map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	return c.listPoolMembers(ctx, lc, poolID)
}

// CreatePoolOpts mirrors DataLoadBalancer.LbPool (the CREATE_POOL action body). The pool is
// keyed to a listener (listener_id), not directly to the LB.
type CreatePoolOpts struct {
	Name        string
	Protocol    string
	ListenerID  string
	LbAlgorithm string
}

// CreatePool creates a pool on a listener.
func (c *Client) CreatePool(ctx context.Context, o CreatePoolOpts) (map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	p, err := pools.Create(ctx, lc, pools.CreateOpts{
		Name:       o.Name,
		Protocol:   pools.Protocol(o.Protocol),
		ListenerID: o.ListenerID,
		LBMethod:   pools.LBMethod(o.LbAlgorithm),
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(p), nil
}

// DeletePool removes a pool by id.
func (c *Client) DeletePool(ctx context.Context, poolID string) error {
	lc, err := c.lbSub()
	if err != nil {
		return err
	}
	return pools.Delete(ctx, lc, poolID).ExtractErr()
}

// AddMemberOpts mirrors DataLoadBalancer.LbMember (the ADD_MEMBER action body). The
// target server's addressIPv4 is resolved from the cached CloudResource (DataServer) before creating
// the member; the caller passes the already-resolved Address here.
type AddMemberOpts struct {
	PoolID  string
	Address string // resolved DataServer.addressIPv4 of the targetId server
	Port    int    // DataLoadBalancer.LbMember.memberPort
}

// AddMember adds a member to a pool.
func (c *Client) AddMember(ctx context.Context, o AddMemberOpts) (map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	m, err := pools.CreateMember(ctx, lc, o.PoolID, pools.CreateMemberOpts{
		Address:      o.Address,
		ProtocolPort: o.Port,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(m), nil
}

// DeleteMember removes a member from a pool.
func (c *Client) DeleteMember(ctx context.Context, poolID, memberID string) error {
	lc, err := c.lbSub()
	if err != nil {
		return err
	}
	return pools.DeleteMember(ctx, lc, poolID, memberID).ExtractErr()
}

// ---- health monitors (GET_MONITORS / ADD_MONITOR / DELETE_MONITOR) ----

// GetMonitors lists the project's health monitors.
func (c *Client) GetMonitors(ctx context.Context) ([]map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	pages, err := monitors.List(lc, monitors.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ms, err := monitors.ExtractMonitors(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ms))
	for i := range ms {
		out = append(out, toMap(ms[i]))
	}
	return out, nil
}

// AddMonitorOpts mirrors DataLoadBalancer.LbMonitoring (the ADD_MONITOR action body). createMonitor
// maps protocol→type and passes the HTTP-only fields straight through.
type AddMonitorOpts struct {
	PoolID         string
	Name           string
	Protocol       string // → healthmonitor type (HTTP/HTTPS/TCP/PING/…)
	Timeout        int
	Delay          int
	MaxRetries     int
	MaxRetriesDown int
	URLPath        string
	HTTPMethod     string
	ExpectedCodes  string
}

// AddMonitor creates a health monitor on a pool.
func (c *Client) AddMonitor(ctx context.Context, o AddMonitorOpts) (map[string]any, error) {
	lc, err := c.lbSub()
	if err != nil {
		return nil, err
	}
	m, err := monitors.Create(ctx, lc, monitors.CreateOpts{
		PoolID:         o.PoolID,
		Name:           o.Name,
		Type:           o.Protocol,
		Timeout:        o.Timeout,
		Delay:          o.Delay,
		MaxRetries:     o.MaxRetries,
		MaxRetriesDown: o.MaxRetriesDown,
		URLPath:        o.URLPath,
		HTTPMethod:     o.HTTPMethod,
		ExpectedCodes:  o.ExpectedCodes,
	}).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(m), nil
}

// DeleteMonitor removes a health monitor by id.
func (c *Client) DeleteMonitor(ctx context.Context, monitorID string) error {
	lc, err := c.lbSub()
	if err != nil {
		return err
	}
	return monitors.Delete(ctx, lc, monitorID).ExtractErr()
}
