package client

import (
	"context"
	"fmt"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/resourcetypes"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stackevents"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stackresources"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacks"
	"github.com/gophercloud/gophercloud/v2/openstack/orchestration/v1/stacktemplates"
)

// stack_actions.go = the Heat (orchestration) ACTIONS surface on the CloudClient facade. The base STACK
// CRUD lives in
// stack.go; this file covers the per-stack actions (DataStack.Action: events/resources/template/
// update-template/suspend/resume/cancel/check/abandon) and the collection-level direct actions
// (DataStack.DirectAction: template-versions/template-functions/resource-types/preview).
//
// Heat is NAME+ID keyed — every per-stack action takes the cached data.stack.name + externalId.
// gophercloud's orchestration package has no helpers for the suspend/resume/check/cancel/abandon
// stack actions, the get-template, or the template-update — those use the direct-REST path over the
// tenant-scoped orchestration ServiceClient (c.provider.Request, mirroring GetVNCConsole in
// client.go). Each method returns a free-form map[string]any / []map[string]any so the SDK type
// never leaks past the facade; pure-action calls that return nothing return error.

// ListStackEvents lists a stack's events (heat events list).
func (c *Client) ListStackEvents(ctx context.Context, stackName, stackID string) ([]map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	pages, err := stackevents.List(hc, stackName, stackID, stackevents.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	evs, err := stackevents.ExtractEvents(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(evs))
	for i := range evs {
		out = append(out, toMap(evs[i]))
	}
	return out, nil
}

// ListStackResources lists a stack's resources (heat resources list).
func (c *Client) ListStackResources(ctx context.Context, stackName, stackID string) ([]map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	pages, err := stackresources.List(hc, stackName, stackID, stackresources.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := stackresources.ExtractResources(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(rs))
	for i := range rs {
		out = append(out, toMap(rs[i]))
	}
	return out, nil
}

// GetStackTemplate fetches a stack's template as a map (heat get-template-as-map; we return the raw
// template map and let the caller render it).
func (c *Client) GetStackTemplate(ctx context.Context, stackName, stackID string) (map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	tpl, err := stacktemplates.Get(ctx, hc, stackName, stackID).Extract() // []byte (raw JSON template)
	if err != nil {
		return nil, err
	}
	return map[string]any{"template": string(tpl)}, nil
}

// UpdateStackOpts mirrors the UPDATE_TEMPLATE payload: a raw HOT/CFN template body
// (+ optional environment).
type UpdateStackOpts struct {
	Template    string
	Environment string
}

// UpdateStackTemplate updates a stack's template (heat stacks update).
// PUT replaces the stack template; Heat returns no body (202).
func (c *Client) UpdateStackTemplate(ctx context.Context, stackName, stackID string, o UpdateStackOpts) error {
	hc, err := c.orchestration()
	if err != nil {
		return err
	}
	if o.Template == "" {
		return fmt.Errorf("template is required")
	}
	opts := stacks.UpdateOpts{
		TemplateOpts: &stacks.Template{TE: stacks.TE{Bin: []byte(o.Template)}},
	}
	if o.Environment != "" {
		opts.EnvironmentOpts = &stacks.Environment{TE: stacks.TE{Bin: []byte(o.Environment)}}
	}
	return stacks.Update(ctx, hc, stackName, stackID, opts).ExtractErr()
}

// stackAction POSTs a Heat stack action body to /stacks/{name}/{id}/actions (the gophercloud
// orchestration package has no typed helper for these). Covers the SUSPEND/RESUME/
// CHECK/CANCEL_UPDATE and cancel-without-rollback actions. Returns nothing on success.
func (c *Client) stackAction(ctx context.Context, hc *gophercloud.ServiceClient, stackName, stackID string, body map[string]any) error {
	url := hc.ServiceURL("stacks", stackName, stackID, "actions")
	_, err := hc.Post(ctx, url, body, nil, &gophercloud.RequestOpts{OkCodes: []int{200, 201, 202, 204}})
	return err
}

// SuspendStack suspends a stack (action SUSPEND).
func (c *Client) SuspendStack(ctx context.Context, stackName, stackID string) error {
	hc, err := c.orchestration()
	if err != nil {
		return err
	}
	return c.stackAction(ctx, hc, stackName, stackID, map[string]any{"suspend": nil})
}

// ResumeStack resumes a stack (action RESUME).
func (c *Client) ResumeStack(ctx context.Context, stackName, stackID string) error {
	hc, err := c.orchestration()
	if err != nil {
		return err
	}
	return c.stackAction(ctx, hc, stackName, stackID, map[string]any{"resume": nil})
}

// CheckStack runs the stack check action (action CHECK).
func (c *Client) CheckStack(ctx context.Context, stackName, stackID string) error {
	hc, err := c.orchestration()
	if err != nil {
		return err
	}
	return c.stackAction(ctx, hc, stackName, stackID, map[string]any{"check": nil})
}

// CancelUpdateStack cancels an in-progress stack update, rolling back (action CANCEL_UPDATE).
func (c *Client) CancelUpdateStack(ctx context.Context, stackName, stackID string) error {
	hc, err := c.orchestration()
	if err != nil {
		return err
	}
	return c.stackAction(ctx, hc, stackName, stackID, map[string]any{"cancel_update": nil})
}

// CancelStackWithoutRollback cancels an in-progress stack update WITHOUT rolling back
// (action cancel_without_rollback).
func (c *Client) CancelStackWithoutRollback(ctx context.Context, stackName, stackID string) error {
	hc, err := c.orchestration()
	if err != nil {
		return err
	}
	return c.stackAction(ctx, hc, stackName, stackID, map[string]any{"cancel_without_rollback": nil})
}

// AbandonStack abandons a stack — deletes the stack record but leaves its resources intact, and
// returns the adopt data describing the stack + resources. Uses gophercloud's stacks.Abandon helper.
func (c *Client) AbandonStack(ctx context.Context, stackName, stackID string) (map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	data, err := stacks.Abandon(ctx, hc, stackName, stackID).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(data), nil
}

// --- collection-level direct actions (DataStack.DirectAction) ---

// ListTemplateVersions lists the supported Heat template versions. Direct-REST: no
// gophercloud helper for /template_versions.
func (c *Client) ListTemplateVersions(ctx context.Context) ([]map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	var resp struct {
		TemplateVersions []map[string]any `json:"template_versions"`
	}
	if _, err := hc.Get(ctx, hc.ServiceURL("template_versions"), &resp, nil); err != nil {
		return nil, err
	}
	return resp.TemplateVersions, nil
}

// ListTemplateFunctions lists the functions available for a given template version. Direct-REST.
func (c *Client) ListTemplateFunctions(ctx context.Context, templateVersion string) ([]map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	if templateVersion == "" {
		return nil, fmt.Errorf("templateVersion is required")
	}
	var resp struct {
		TemplateFunctions []map[string]any `json:"template_functions"`
	}
	url := hc.ServiceURL("template_versions", templateVersion, "functions")
	if _, err := hc.Get(ctx, url, &resp, nil); err != nil {
		return nil, err
	}
	return resp.TemplateFunctions, nil
}

// ListResourceTypes lists the registered Heat resource types. Returns the resource-type names.
func (c *Client) ListResourceTypes(ctx context.Context) ([]string, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	rts, err := resourcetypes.List(ctx, hc, resourcetypes.ListOpts{}).Extract()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rts))
	for _, rt := range rts {
		out = append(out, rt.ResourceType)
	}
	return out, nil
}

// PreviewStackOpts mirrors the PREVIEW_STACK payload: name + raw template (+ optional environment)
// + disableRollback.
type PreviewStackOpts struct {
	Name            string
	Template        string
	Environment     string
	DisableRollback bool
}

// PreviewStack previews the resources a stack would create without creating it
// (heat stacks preview). gophercloud's stacks.Preview requires Timeout, so
// the previewed stack carries a 60-minute placeholder timeout.
func (c *Client) PreviewStack(ctx context.Context, o PreviewStackOpts) (map[string]any, error) {
	hc, err := c.orchestration()
	if err != nil {
		return nil, err
	}
	if o.Template == "" {
		return nil, fmt.Errorf("template is required")
	}
	rollback := o.DisableRollback
	opts := stacks.PreviewOpts{
		Name:            o.Name,
		Timeout:         60,
		TemplateOpts:    &stacks.Template{TE: stacks.TE{Bin: []byte(o.Template)}},
		DisableRollback: &rollback,
	}
	if o.Environment != "" {
		opts.EnvironmentOpts = &stacks.Environment{TE: stacks.TE{Bin: []byte(o.Environment)}}
	}
	preview, err := stacks.Preview(ctx, hc, opts).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(preview), nil
}
