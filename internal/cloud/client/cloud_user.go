package client

import (
	"context"
	cryptorand "crypto/rand"
	"fmt"
	"math/big"
	"strings"

	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/identity/v3/users"
)

// cloud_user.go = the Keystone (identity v3) USER cloud-resource read/write surface on the CloudClient
// facade. An
// API user is the USER CloudResource; its externalId == the Keystone user id. data = DataUser
// {username, description}; the generated password is ephemeral (returned once at create, never stored).
//
// NOTE: the bootstrap-time admin identity ops (CreateProject/FindUserID/FindRoleID/GrantProjectUserRole)
// already live in identity.go on this same *Client and are NOT duplicated here. This file adds only the
// USER CloudResource CRUD + the GENERATE_PASSWORD action that the cloud-resource provider needs.

func (c *Client) identity() (*gophercloud.ServiceClient, error) {
	return openstack.NewIdentityV3(c.provider, c.endpointOpts())
}

// randomAlphanumeric returns a length-n string of [0-9A-Za-z]
// (used for the username suffix + the generated password).
func randomAlphanumeric(n int) string {
	const charset = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	b := make([]byte, n)
	for i := range b {
		idx, err := cryptorand.Int(cryptorand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// crypto/rand should never fail; fall back to the first char to avoid a panic.
			b[i] = charset[0]
			continue
		}
		b[i] = charset[idx.Int64()]
	}
	return string(b)
}

// CreateUserOpts holds the Keystone-user create fields derived from the create request + service
// context: the username is suffixed with 4 random alphanumerics + lowercased, and a 16-char password
// is generated server-side. Pass the resolved DomainID/DefaultProjectID/Email from the caller.
type CreateUserOpts struct {
	Username         string // base name (the provider appends "-<4 alnum>" + lowercases)
	Description      string
	Email            string
	DomainID         string // openstackConfig.customer.domainId
	DefaultProjectID string // the project's externalProjectId
	Enabled          *bool  // nil → true
}

// CreatedUser is what CreateUser returns: the created Keystone user as a free-form map (the
// CloudResource.data.user shape, with id/name) plus the one-time generated password (kept on
// ephemeral data and never persisted). The SDK type never leaks.
type CreatedUser struct {
	User     map[string]any
	Username string
	Password string
}

// CreateUser creates a Keystone (API) user: name = "<username>-<4 alnum>" lowercased, a freshly
// generated 16-char password, domainId/defaultProjectId/email from the service context, enabled=true.
// Role assignment to the project is wired separately (see GrantProjectUserRole in
// identity.go) — this method only creates the user and surfaces the ephemeral password.
func (c *Client) CreateUser(ctx context.Context, o CreateUserOpts) (*CreatedUser, error) {
	ic, err := c.identity()
	if err != nil {
		return nil, err
	}
	username := strings.ToLower(fmt.Sprintf("%s-%s", o.Username, randomAlphanumeric(4)))
	password := randomAlphanumeric(16)
	enabled := true
	if o.Enabled != nil {
		enabled = *o.Enabled
	}
	opts := users.CreateOpts{
		Name:             username,
		Description:      o.Description,
		DomainID:         o.DomainID,
		DefaultProjectID: o.DefaultProjectID,
		Password:         password,
		Enabled:          &enabled,
	}
	if o.Email != "" {
		// Keystone has no first-class email field on a user; it is sent
		// as an extra attribute. gophercloud exposes that via CreateOpts.Extra.
		opts.Extra = map[string]any{"email": o.Email}
	}
	u, err := users.Create(ctx, ic, opts).Extract()
	if err != nil {
		return nil, err
	}
	return &CreatedUser{User: toMap(u), Username: username, Password: password}, nil
}

// GetUser fetches a Keystone user by id.
func (c *Client) GetUser(ctx context.Context, id string) (map[string]any, error) {
	ic, err := c.identity()
	if err != nil {
		return nil, err
	}
	u, err := users.Get(ctx, ic, id).Extract()
	if err != nil {
		return nil, err
	}
	return toMap(u), nil
}

// ListUsers lists the Keystone users visible to the scoped client.
func (c *Client) ListUsers(ctx context.Context) ([]map[string]any, error) {
	ic, err := c.identity()
	if err != nil {
		return nil, err
	}
	pages, err := users.List(ic, users.ListOpts{}).AllPages(ctx)
	if err != nil {
		return nil, err
	}
	us, err := users.ExtractUsers(pages)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, 0, len(us))
	for i := range us {
		out = append(out, toMap(us[i]))
	}
	return out, nil
}

// DeleteUser removes a Keystone user by id.
func (c *Client) DeleteUser(ctx context.Context, id string) error {
	ic, err := c.identity()
	if err != nil {
		return err
	}
	return users.Delete(ctx, ic, id).ExtractErr()
}

// GeneratePassword resets a Keystone user's password to a fresh 16-char alphanumeric and returns it
// (the GENERATE_PASSWORD action). Returns the new password (the caller surfaces it as a one-time value).
func (c *Client) GeneratePassword(ctx context.Context, userID string) (string, error) {
	ic, err := c.identity()
	if err != nil {
		return "", err
	}
	password := randomAlphanumeric(16)
	if _, err := users.Update(ctx, ic, userID, users.UpdateOpts{Password: password}).Extract(); err != nil {
		return "", err
	}
	return password, nil
}
