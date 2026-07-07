//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/menlocloud/stratos/internal/platform/externalservice"
	"github.com/menlocloud/stratos/pkg/textcrypt"
)

// TestExternalServiceDecryptRoundTrip verifies the Track-1.2 path against real Postgres: a doc
// whose `secret` is AES-encrypted (per textual leaf, including a nested object) decodes
// as JSONB sub-documents, is normalized to plain maps, and is decrypted in place by the
// Service. It also checks the OpenStack config accessors + ClientConfig mapping.
func TestExternalServiceDecryptRoundTrip(t *testing.T) {
	ctx := context.Background()
	db := freshPG(t)

	const key = "dev-359ac834f15e8e013db24c77b6b3e64c"
	enc := textcrypt.New(key)

	// Encrypt the secret the way EncryptObject does (each textual leaf), then store.
	encryptedSecret := enc.EncryptObject(map[string]any{
		"adminPassword":               "s3cr3t-pw",
		"applicationCredentialSecret": "appcred-secret",
		"vhiOstorAuth":                map[string]any{"accessKey": "AK", "secretKey": "SK"},
	})

	doc := map[string]any{
		"_id":    "svc-openstack-1",
		"name":   "dev region",
		"type":   externalservice.TypeCloud,
		"status": externalservice.StatusPublic,
		"config": map[string]any{
			"identityUrl":        "https://cloud-console.menlo.ai:5000", // no /v3 → normalized
			"provider":           "openstack",
			"shared":             false,
			"gnocchiGranularity": 600,
			"regions":            map[string]any{"RegionOne": map[string]any{}},
			"auth": map[string]any{
				"adminAuthType":   "password",
				"adminUsername":   "dev",
				"adminProjectId":  "proj-uuid",
				"adminDomainName": "Default",
			},
		},
		"secret": encryptedSecret,
	}
	if _, err := db.C("externalService").InsertOne(ctx, doc); err != nil {
		t.Fatalf("seed externalService: %v", err)
	}

	svc := externalservice.NewService(externalservice.NewRepo(db), enc)

	es, err := svc.Get(ctx, "svc-openstack-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}

	// secret decrypted in place (nested object too).
	secret, ok := es.Secret.(map[string]any)
	if !ok {
		t.Fatalf("secret not a map: %T", es.Secret)
	}
	if secret["adminPassword"] != "s3cr3t-pw" {
		t.Fatalf("adminPassword not decrypted: %v", secret["adminPassword"])
	}
	if secret["applicationCredentialSecret"] != "appcred-secret" {
		t.Fatalf("applicationCredentialSecret not decrypted: %v", secret["applicationCredentialSecret"])
	}
	nested, ok := secret["vhiOstorAuth"].(map[string]any)
	if !ok || nested["accessKey"] != "AK" || nested["secretKey"] != "SK" {
		t.Fatalf("nested vhiOstorAuth not decrypted: %#v", secret["vhiOstorAuth"])
	}

	// config accessors.
	if got := es.IdentityURL(); got != "https://cloud-console.menlo.ai:5000/v3" {
		t.Fatalf("identityUrl not normalized to /v3: %q", got)
	}
	if es.GnocchiGranularity() != 600 {
		t.Fatalf("gnocchiGranularity: got %d want 600", es.GnocchiGranularity())
	}

	// ClientConfig (password auth) maps decrypted creds + project-id scope.
	cc := es.ClientConfig("RegionOne")
	if cc.AuthURL != "https://cloud-console.menlo.ai:5000/v3" || cc.Username != "dev" ||
		cc.Password != "s3cr3t-pw" || cc.ProjectID != "proj-uuid" || cc.UserDomainName != "Default" || cc.Region != "RegionOne" {
		t.Fatalf("ClientConfig mapping wrong: %+v", cc)
	}

	// List path decrypts too.
	all, err := svc.List(ctx)
	if err != nil || len(all) != 1 {
		t.Fatalf("list: %v len=%d", err, len(all))
	}
	if m := all[0].Secret.(map[string]any); m["adminPassword"] != "s3cr3t-pw" {
		t.Fatalf("list did not decrypt: %v", m["adminPassword"])
	}
}
