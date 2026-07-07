package client

import (
	"context"
	"strings"
	"time"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/keymanager/v1/secrets"
)

// barbican.go = the Barbican (key-manager) secret read/write surface on the CloudClient facade. A secret
// is the BARBICAN_SECRET CloudResource; its externalId
// is the UUID tail of the secret_ref URL. Each call returns the object as a free-form map[string]any
// (the CloudResource.data.secret shape) with an injected "id", so the SDK type never leaks.

func (c *Client) keymanager() (*gophercloud.ServiceClient, error) {
	return openstack.NewKeyManagerV1(c.provider, c.endpointOpts())
}

// secretIDFromRef extracts the UUID tail of a Barbican secret_ref URL (the last path segment).
func secretIDFromRef(ref string) string {
	ref = strings.TrimRight(ref, "/")
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		return ref[i+1:]
	}
	return ref
}

// secretToMap renders a Secret as the data.secret map with an injected bare "id" (extracted from the
// secret_ref) — the FE/list/detail key the cache resolves by.
func secretToMap(s *secrets.Secret) map[string]any {
	m := toMap(s)
	m["id"] = secretIDFromRef(s.SecretRef)
	return m
}

// CreateSecretOpts mirrors CreateBarbicanSecretRequest (the FE create body).
type CreateSecretOpts struct {
	Name                   string
	SecretType             string
	Algorithm              string
	BitLength              int
	Mode                   string
	Expiration             string
	PayloadContentType     string
	PayloadContentEncoding string
	Payload                string
}

// CreateSecret creates a Barbican secret, then re-fetches it (create → get) so
// the cached data carries the full, server-populated secret. Returns the data.secret map.
func (c *Client) CreateSecret(ctx context.Context, o CreateSecretOpts) (map[string]any, error) {
	kc, err := c.keymanager()
	if err != nil {
		return nil, err
	}
	opts := secrets.CreateOpts{
		Name:                   o.Name,
		Algorithm:              o.Algorithm,
		BitLength:              o.BitLength,
		Mode:                   o.Mode,
		Payload:                o.Payload,
		PayloadContentType:     o.PayloadContentType,
		PayloadContentEncoding: o.PayloadContentEncoding,
		SecretType:             secrets.SecretType(o.SecretType),
	}
	if o.Expiration != "" {
		t, perr := time.Parse(time.RFC3339, o.Expiration)
		if perr != nil {
			return nil, perr
		}
		opts.Expiration = &t
	}
	created, err := secrets.Create(ctx, kc, opts).Extract()
	if err != nil {
		return nil, err
	}
	id := secretIDFromRef(created.SecretRef)
	// Re-read the secret after create; fall back to the create response if the read fails.
	if full, gerr := secrets.Get(ctx, kc, id).Extract(); gerr == nil && full != nil {
		return secretToMap(full), nil
	}
	return secretToMap(created), nil
}

// GetSecret fetches a Barbican secret by UUID.
func (c *Client) GetSecret(ctx context.Context, id string) (map[string]any, error) {
	kc, err := c.keymanager()
	if err != nil {
		return nil, err
	}
	s, err := secrets.Get(ctx, kc, id).Extract()
	if err != nil {
		return nil, err
	}
	return secretToMap(s), nil
}

// ListSecrets lists the project's Barbican secrets.
func (c *Client) ListSecrets(ctx context.Context) ([]map[string]any, error) {
	kc, err := c.keymanager()
	if err != nil {
		return nil, err
	}
	pages, err := secrets.List(kc, secrets.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	ss, err := secrets.ExtractSecrets(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(ss))
	for i := range ss {
		out = append(out, secretToMap(&ss[i]))
	}
	return out, nil
}

// DeleteSecret removes a Barbican secret by UUID.
func (c *Client) DeleteSecret(ctx context.Context, id string) error {
	kc, err := c.keymanager()
	if err != nil {
		return err
	}
	return secrets.Delete(ctx, kc, id).ExtractErr()
}
