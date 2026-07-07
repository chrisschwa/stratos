package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// niche_sync.go adds read-sync providers for the TOKEN-SCOPED niche cloud types — Barbican secrets,
// Swift buckets, Designate DNS zones — so their cache stops drifting (the syncjob previously synced
// only server/port/volume/floating_ip/load_balancer). These are
// SAFE to sync without an explicit project filter because the syncjob's client is project-token-scoped:
// barbican/swift/designate return only THIS project's resources. (The neutron types — network/router/
// subnet/security_group — are deliberately NOT added here: a project-scoped neutron token still lists
// EVERY tenant unless `project_id` is passed, so syncing them unscoped would re-introduce the
// dev160-161 cross-tenant leak; they need per-type project filtering + a live run-sync to verify.)
//
// Each provider's Data shape MIRRORS refreshResource (cloud_writes.go) exactly, so reconciling an
// unchanged resource produces no spurious "differs" → no write/audit churn.
//
// ⚠ CODE + UNIT ONLY — NOT live-verified: the shared dev region is READ-ONLY this session, so a real
// run-sync could not exercise these. The pure list→CloudResource mapping is unit-tested; the wiring
// mirrors the existing read-sync providers (e.g. FloatingIPProvider). Run `/debug/run-sync` to confirm
// before relying on the delete-of-vanished behaviour.

// --- Barbican secret ---

type BarbicanSecretProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewBarbicanSecretProvider(cc *client.Client, region, projectID string) *BarbicanSecretProvider {
	return &BarbicanSecretProvider{cc: cc, region: region, projectID: projectID}
}

func (p *BarbicanSecretProvider) Type() string          { return cloud.TypeBarbicanSecret }
func (p *BarbicanSecretProvider) ProjectID() string     { return p.projectID }
func (p *BarbicanSecretProvider) CompareKeys() []string { return []string{"secret"} }

func (p *BarbicanSecretProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	ss, err := p.cc.ListSecrets(ctx)
	if err != nil {
		return nil, err
	}
	return secretsToResources(ss, p.region, p.projectID), nil
}

// secretsToResources mirrors refreshResource's BARBICAN_SECRET shape: externalId = the secret UUID,
// data = {"secret": <secretToMap>}.
func secretsToResources(ss []map[string]any, region, projectID string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(ss))
	for _, s := range ss {
		id, _ := s["id"].(string)
		if id == "" {
			continue
		}
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeBarbicanSecret, ExternalID: id, Region: region, ProjectID: projectID,
			Data: map[string]any{"secret": s},
		})
	}
	return out
}

// --- Swift bucket ---

type BucketProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewBucketProvider(cc *client.Client, region, projectID string) *BucketProvider {
	return &BucketProvider{cc: cc, region: region, projectID: projectID}
}

func (p *BucketProvider) Type() string      { return cloud.TypeBucket }
func (p *BucketProvider) ProjectID() string { return p.projectID }

// CompareKeys empty = whole-map compareMaps (the object-store provider passes a null dataKey). The
// bucket data is FLAT (no wrapping key); whole-map compare with number tolerance stops the numeric
// objectCount/sizeInBytes from churning on a JSON round-trip.
func (p *BucketProvider) CompareKeys() []string { return []string{} }

func (p *BucketProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	bs, err := p.cc.ListBuckets(ctx)
	if err != nil {
		return nil, err
	}
	return bucketsToResources(bs, p.region, p.projectID), nil
}

// bucketsToResources mirrors refreshResource's BUCKET shape: externalId = the bucket name, data = the
// flat DataBucket map (cr.Data = b, NOT wrapped).
func bucketsToResources(bs []map[string]any, region, projectID string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(bs))
	for _, b := range bs {
		name, _ := b["bucketName"].(string)
		if name == "" {
			continue
		}
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeBucket, ExternalID: name, Region: region, ProjectID: projectID, Data: b,
		})
	}
	return out
}

// --- Designate DNS zone ---

type DNSZoneProvider struct {
	cc        *client.Client
	region    string
	projectID string
}

func NewDNSZoneProvider(cc *client.Client, region, projectID string) *DNSZoneProvider {
	return &DNSZoneProvider{cc: cc, region: region, projectID: projectID}
}

func (p *DNSZoneProvider) Type() string          { return cloud.TypeDNSZone }
func (p *DNSZoneProvider) ProjectID() string     { return p.projectID }
func (p *DNSZoneProvider) CompareKeys() []string { return []string{"zone"} }

func (p *DNSZoneProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	zs, err := p.cc.ListZones(ctx)
	if err != nil {
		return nil, err
	}
	return zonesToResources(zs, p.region, p.projectID), nil
}

// zonesToResources mirrors the DNS_ZONE create/notification shape: externalId = the zone UUID, data =
// {"zone": <zone>, "name": <zone.name>}.
func zonesToResources(zs []map[string]any, region, projectID string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(zs))
	for _, z := range zs {
		id, _ := z["id"].(string)
		if id == "" {
			continue
		}
		name, _ := z["name"].(string)
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeDNSZone, ExternalID: id, Region: region, ProjectID: projectID,
			Data: map[string]any{"zone": z, "name": name},
		})
	}
	return out
}
