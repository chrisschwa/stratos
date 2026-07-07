package client

import (
	"context"

	"github.com/gophercloud/gophercloud/v2/openstack/keymanager/v1/containers"
)

// barbican_container.go = the Barbican (key-manager) secret-CONTAINER read/write surface on the
// CloudClient facade (distinct from the secret provider in barbican.go). A container is the
// BARBICAN_CONTAINER CloudResource; its externalId is the UUID
// tail of the container_ref URL (mirrors secretIDFromRef). Each call returns the object as a free-form
// map[string]any (the CloudResource.data.container shape) with an injected bare "id", so the SDK type
// never leaks. Only "generic" containers can have their secrets mutated.

// containerToMap renders a Container as the data.container map with an injected bare "id" extracted
// from the container_ref.
func containerToMap(c *containers.Container) map[string]any {
	m := toMap(c)
	m["id"] = secretIDFromRef(c.ContainerRef)
	return m
}

// ContainerSecretRef mirrors BarbicanContainer.SecretRef {name, secretRef} (a named pointer to a
// Barbican secret inside a generic container).
type ContainerSecretRef struct {
	Name      string
	SecretRef string
}

// CreateContainerOpts mirrors CreateBarbicanContainerRequest (the FE create body).
type CreateContainerOpts struct {
	Name       string
	Type       string // "generic" | "rsa" | "certificate"
	SecretRefs []ContainerSecretRef
}

// CreateContainer creates a Barbican container, then re-fetches it (create → get)
// so the cached data carries the full, server-populated container. Returns the data.container map.
func (c *Client) CreateContainer(ctx context.Context, o CreateContainerOpts) (map[string]any, error) {
	kc, err := c.keymanager()
	if err != nil {
		return nil, err
	}
	ctype := containers.ContainerType(o.Type)
	if ctype == "" {
		ctype = containers.GenericContainer
	}
	opts := containers.CreateOpts{
		Type: ctype,
		Name: o.Name,
	}
	for _, ref := range o.SecretRefs {
		opts.SecretRefs = append(opts.SecretRefs, containers.SecretRef{
			Name:      ref.Name,
			SecretRef: ref.SecretRef,
		})
	}
	created, err := containers.Create(ctx, kc, opts).Extract()
	if err != nil {
		return nil, err
	}
	id := secretIDFromRef(created.ContainerRef)
	// Re-read the container after create; fall back to the create response if the read fails.
	if full, gerr := containers.Get(ctx, kc, id).Extract(); gerr == nil && full != nil {
		return containerToMap(full), nil
	}
	return containerToMap(created), nil
}

// GetContainer fetches a Barbican container by UUID.
func (c *Client) GetContainer(ctx context.Context, id string) (map[string]any, error) {
	kc, err := c.keymanager()
	if err != nil {
		return nil, err
	}
	cnt, err := containers.Get(ctx, kc, id).Extract()
	if err != nil {
		return nil, err
	}
	return containerToMap(cnt), nil
}

// ListContainers lists the project's Barbican containers.
func (c *Client) ListContainers(ctx context.Context) ([]map[string]any, error) {
	kc, err := c.keymanager()
	if err != nil {
		return nil, err
	}
	pages, err := containers.List(kc, containers.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	cs, err := containers.ExtractContainers(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(cs))
	for i := range cs {
		out = append(out, containerToMap(&cs[i]))
	}
	return out, nil
}

// DeleteContainer removes a Barbican container by UUID.
func (c *Client) DeleteContainer(ctx context.Context, id string) error {
	kc, err := c.keymanager()
	if err != nil {
		return err
	}
	return containers.Delete(ctx, kc, id).ExtractErr()
}

// AddContainerSecret adds a named secret reference to a generic container
// (POST /containers/{id}/secrets {name, secret_ref}). gophercloud CreateSecretRef hits the same URL/body.
func (c *Client) AddContainerSecret(ctx context.Context, containerID, name, secretRef string) error {
	kc, err := c.keymanager()
	if err != nil {
		return err
	}
	return containers.CreateSecretRef(ctx, kc, containerID, containers.SecretRef{
		Name:      name,
		SecretRef: secretRef,
	}).Err
}

// RemoveContainerSecret removes a named secret reference from a generic container
// (DELETE /containers/{id}/secrets {name, secret_ref}).
func (c *Client) RemoveContainerSecret(ctx context.Context, containerID, name, secretRef string) error {
	kc, err := c.keymanager()
	if err != nil {
		return err
	}
	return containers.DeleteSecretRef(ctx, kc, containerID, containers.SecretRef{
		Name:      name,
		SecretRef: secretRef,
	}).Err
}
