package client

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
)

// stack.go = the Heat (orchestration) stack read/write surface on the CloudClient facade. A stack is the
// STACK CloudResource; externalId == the
// Heat stack id. Heat is NAME+ID keyed (get/delete take both), so the cached data.stack.name is
// needed to delete. data = DataStack{stack}.

func (c *Client) orchestration() (*gophercloud.ServiceClient, error) {
	return openstack.NewOrchestrationV1(c.provider, c.endpointOpts())
}

// CreateStackOpts mirrors the base CreateStackRequest: a
// raw HOT/CFN template body (+ optional environment). templateUrl/file sources are not handled here
// (the FE inline-template path is the common one).
type CreateStackOpts struct {
	Name            string
	Template        string
	Environment     string
	DisableRollback bool
}

// CreateStack creates a Heat stack then re-fetches it by (name,id) (create → get).
func (c *Client) CreateStack(ctx context.Context, o CreateStackOpts) (map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	if o.Template == "" {
		return nil, fmt.Errorf("template is required")
	}
	rollback := o.DisableRollback
	opts := stacks.CreateOpts{
		Name:            o.Name,
		TemplateOpts:    &stacks.Template{TE: stacks.TE{Bin: []byte(o.Template)}},
		DisableRollback: &rollback,
	}
	if o.Environment != "" {
		opts.EnvironmentOpts = &stacks.Environment{TE: stacks.TE{Bin: []byte(o.Environment)}}
	}
	created, err := stacks.Create(ctx, hc, opts).Extract()
	if err != nil {
		return nil, err
	}
	// Re-read the full stack; fall back to a minimal {id,name} map if the read fails.
	if full, gerr := stacks.Get(ctx, hc, o.Name, created.ID).Extract(); gerr == nil && full != nil {
		return toMap(full), nil
	}
	return map[string]any{"id": created.ID, "stack_name": o.Name}, nil
}

// GetStack fetches a Heat stack by (name, id).
func (c *Client) GetStack(ctx context.Context, name, id string) (map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	s, err := stacks.Get(ctx, hc, name, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(s), nil
}

// DeleteStack deletes a Heat stack by (name, id).
func (c *Client) DeleteStack(ctx context.Context, name, id string) error {
	hc, err := c.orchestration()
	if err != nil {
		return err
	}
	return stacks.Delete(ctx, hc, name, id).ExtractErr()
}
