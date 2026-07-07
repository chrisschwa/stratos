package providers

import (
	"testing"

	"github.com/menlocloud/stratos/internal/cloud"
)

func TestSecretsToResources(t *testing.T) {
	in := []map[string]any{
		{"id": "sec-1", "name": "pw-secret", "status": "ACTIVE"},
		{"name": "no-id"}, // skipped — no id
	}
	got := secretsToResources(in, "RegionOne", "proj-1")
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d: %#v", len(got), got)
	}
	r := got[0]
	if r.Type != cloud.TypeBarbicanSecret || r.ExternalID != "sec-1" || r.Region != "RegionOne" || r.ProjectID != "proj-1" {
		t.Errorf("bad resource: %#v", r)
	}
	if sec, _ := r.Data["secret"].(map[string]any); sec == nil || sec["name"] != "pw-secret" {
		t.Errorf("data.secret mismatch: %#v", r.Data)
	}
}

func TestBucketsToResources(t *testing.T) {
	in := []map[string]any{
		{"bucketName": "pw-bucket", "objectCount": 3, "sizeInGb": 0.0},
		{"objectCount": 1}, // skipped — no bucketName
	}
	got := bucketsToResources(in, "RegionOne", "proj-1")
	if len(got) != 1 || got[0].ExternalID != "pw-bucket" || got[0].Type != cloud.TypeBucket {
		t.Fatalf("got %#v", got)
	}
	// bucket data is the FLAT map (cr.Data = b), not wrapped.
	if got[0].Data["bucketName"] != "pw-bucket" || got[0].Data["objectCount"] != 3 {
		t.Errorf("flat bucket data mismatch: %#v", got[0].Data)
	}
}

func TestZonesToResources(t *testing.T) {
	in := []map[string]any{
		{"id": "zone-1", "name": "pwtest.example.com."},
		{"name": "no-id"}, // skipped
	}
	got := zonesToResources(in, "RegionOne", "proj-1")
	if len(got) != 1 || got[0].ExternalID != "zone-1" || got[0].Type != cloud.TypeDNSZone {
		t.Fatalf("got %#v", got)
	}
	if got[0].Data["name"] != "pwtest.example.com." {
		t.Errorf("zone name not lifted: %#v", got[0].Data)
	}
	if z, _ := got[0].Data["zone"].(map[string]any); z == nil || z["id"] != "zone-1" {
		t.Errorf("data.zone mismatch: %#v", got[0].Data)
	}
}
