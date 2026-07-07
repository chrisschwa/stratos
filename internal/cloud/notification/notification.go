// Package notification is the OpenStack os-notification ingestion path.
// OpenStack/ceilometer HTTP-POSTs an oslo notification to
// /api/v1/notifications/{externalServiceId}/{region}; Stratos routes it by event_type to a
// CloudResourceType, (admin-scoped) re-fetches the live object, and upserts/deletes the
// `cloudResource` cache — keeping the cache eventually-consistent between sync passes.
//
// The fetch (admin-scoped, sudo-to-project) and the project lookup are seams (ResourceFetcher
// / ProjectResolver), so the routing + decision logic is unit-testable without a live cloud,
// mirroring the metrics MeasureFetcher pattern.
package notification

import (
	"context"
	"strings"
	"time"

	"github.com/menlocloud/stratos/internal/cloud"
)

// OsloMessage is the oslo.messaging notification envelope. Field names are
// the wire snake_case keys oslo emits.
type OsloMessage struct {
	MessageID   string         `json:"message_id"`
	EventType   string         `json:"event_type"`
	PublisherID string         `json:"publisher_id"`
	Priority    string         `json:"priority"`
	Timestamp   *time.Time     `json:"timestamp"`
	Payload     map[string]any `json:"payload"`
}

// ResourceFetcher re-reads the live OpenStack object for a resource, admin-scoped + sudo to
// the owning project.
// found=false means the object is gone in OpenStack → the notification resolves to a DELETE.
type ResourceFetcher interface {
	Get(ctx context.Context, externalProjectID, resourceType, externalID string) (obj map[string]any, found bool, err error)
}

// ProjectResolver maps an OpenStack project id → the internal Stratos project id.
// ok=false when the project is unknown.
type ProjectResolver interface {
	ByExternalID(ctx context.Context, externalProjectID string) (projectID string, ok bool)
}

// BareMetalChecker reports whether a nova instance_type names a bare-metal flavor
// — decides SERVER vs BAREMETAL_SERVER. Under
// the greenfield seed the flavorCategory collection is empty, so this is false → SERVER.
type BareMetalChecker func(instanceType string) bool

// Service handles one notification end to end against the cache.
type Service struct {
	repo     *cloud.Repo
	fetch    ResourceFetcher
	projects ProjectResolver
	bareMeta BareMetalChecker
}

func NewService(repo *cloud.Repo, fetch ResourceFetcher, projects ProjectResolver, bareMeta BareMetalChecker) *Service {
	if bareMeta == nil {
		bareMeta = func(string) bool { return false }
	}
	return &Service{repo: repo, fetch: fetch, projects: projects, bareMeta: bareMeta}
}

// minimal is the externalProjectId + externalResourceId extracted from the payload,
// keyed per type by evMeta.
type minimal struct {
	externalResourceID string
	externalProjectID  string
}

// evMeta names, per CloudResourceType, the oslo payload keys: idKey = the flat "<x>_id" field;
// objKey = the nested "<x>" object holding {id, tenant_id}.
// Only the types Stratos acts on are listed.
type evMeta struct{ idKey, objKey string }

var metaByType = map[string]evMeta{
	cloud.TypeServer:            {"instance_id", "instance"},
	cloud.TypeBaremetalServer:   {"instance_id", "instance"},
	cloud.TypeVolume:            {"volume_id", "volume"},
	cloud.TypeNetwork:           {"network_id", "network"},
	cloud.TypeSubnet:            {"subnet_id", "subnet"},
	cloud.TypeRouter:            {"router_id", "router"},
	cloud.TypePort:              {"port_id", "port"},
	cloud.TypeFloatingIP:        {"floatingip_id", "floatingip"},
	cloud.TypeImage:             {"id", "image"}, // glance oslo puts the image id in payload.id (NOT resource_id)
	cloud.TypeKubernetesCluster: {"cluster_id", "cluster"},
	cloud.TypeStack:             {"stack_identity", "stack"},
	cloud.TypeShare:             {"share_id", "share"},
	cloud.TypeDNSZone:           {"id", "zone"}, // designate oslo puts the zone id in payload.id
}

// TypeForEvent maps the first dot-segment of event_type → CloudResourceType.
// ok=false for an unmapped prefix (skip). compute disambiguates
// SERVER vs BAREMETAL_SERVER via the payload instance_type + the bare-metal check.
func TypeForEvent(msg OsloMessage, bareMeta BareMetalChecker) (string, bool) {
	prefix, _, _ := strings.Cut(msg.EventType, ".")
	switch prefix {
	case "compute":
		if it, _ := msg.Payload["instance_type"].(string); it != "" && bareMeta != nil && bareMeta(it) {
			return cloud.TypeBaremetalServer, true
		}
		return cloud.TypeServer, true
	case "image":
		return cloud.TypeImage, true
	case "volume":
		return cloud.TypeVolume, true
	case "dns":
		return cloud.TypeDNSZone, true
	case "network":
		return cloud.TypeNetwork, true
	case "subnet":
		return cloud.TypeSubnet, true
	case "floatingip":
		return cloud.TypeFloatingIP, true
	case "router":
		return cloud.TypeRouter, true
	case "magnum":
		return cloud.TypeKubernetesCluster, true
	case "port":
		return cloud.TypePort, true
	case "security_group":
		return cloud.TypeSecurityGroup, true
	case "orchestration":
		return cloud.TypeStack, true
	case "share":
		return cloud.TypeShare, true
	default:
		return "", false
	}
}

// minimalInfo extracts the minimal ids from the payload: externalResourceId =
// payload["<x>_id"] else payload["<x>"]["id"]; externalProjectId = payload["tenant_id"] else
// payload["<x>"]["tenant_id"] else payload["project_id"].
func minimalInfo(typ string, payload map[string]any) minimal {
	m := metaByType[typ]
	id := strVal(payload, m.idKey)
	if id == "" && m.objKey != "" {
		id = nestedStr(payload, m.objKey, "id")
	}
	proj := strVal(payload, "tenant_id")
	if proj == "" && m.objKey != "" {
		proj = nestedStr(payload, m.objKey, "tenant_id")
	}
	if proj == "" {
		proj = strVal(payload, "project_id")
	}
	return minimal{externalResourceID: id, externalProjectID: proj}
}

// Handle processes one notification message.
func (s *Service) Handle(ctx context.Context, serviceID, region string, msg OsloMessage) error {
	typ, ok := TypeForEvent(msg, s.bareMeta)
	if !ok {
		return nil // unmapped event_type → skip
	}
	info := minimalInfo(typ, msg.Payload)
	if info.externalResourceID == "" {
		return nil // prepareMinimalInfo: blank resource id → skip
	}

	// Resolve the internal project: by external project id, else by a cached resource's project
	// (fallback). No project → skip.
	projectID := ""
	if info.externalProjectID != "" {
		if pid, ok := s.projects.ByExternalID(ctx, info.externalProjectID); ok {
			projectID = pid
		}
	}
	if projectID == "" {
		if cr, err := s.repo.FindByServiceIDAndExternalID(ctx, serviceID, info.externalResourceID); err != nil {
			return err
		} else if cr != nil {
			projectID = cr.ProjectID
		}
	}
	if projectID == "" {
		return nil // unresolvable project → skip
	}

	// processOsNotification: a delete event, or a live object that no longer exists, → DELETE;
	// otherwise re-fetch the object and CREATE_UPDATE the cache with it.
	isDelete := strings.Contains(msg.EventType, "delete")
	var obj map[string]any
	if !isDelete {
		o, found, err := s.fetch.Get(ctx, info.externalProjectID, typ, info.externalResourceID)
		if err != nil {
			return err
		}
		if !found {
			isDelete = true
		} else {
			obj = o
		}
	}

	now := time.Now().UTC()
	if isDelete {
		cr, err := s.repo.FindByServiceIDAndExternalID(ctx, serviceID, info.externalResourceID)
		if err != nil || cr == nil {
			return err
		}
		// Scope the delete to the resolved project: a notification whose tenant resolves to one
		// project must not archive a cached resource owned by another project (a forged/mismatched
		// notification must not delete across the tenant boundary).
		if !sameProject(cr.ProjectID, projectID) {
			return nil
		}
		return s.repo.DeleteAndArchive(ctx, cr, now)
	}

	ts := now
	if msg.Timestamp != nil {
		ts = msg.Timestamp.UTC()
	}
	cr := &cloud.CloudResource{
		ExternalID: info.externalResourceID,
		Type:       typ,
		ProjectID:  projectID,
		ServiceID:  serviceID,
		Region:     region,
		Data:       obj,
		CreatedAt:  &ts,
		UpdatedAt:  &ts,
	}
	_, err := s.repo.Insert(ctx, cr) // upsert (create-or-update)
	return err
}

// sameProject reports whether the cached resource belongs to the notification's resolved project.
// A blank resolved project (should not reach here) is treated as a non-match to fail closed.
func sameProject(resourceProjectID, resolvedProjectID string) bool {
	return resolvedProjectID != "" && resourceProjectID == resolvedProjectID
}

func strVal(m map[string]any, key string) string {
	if m == nil {
		return ""
	}
	s, _ := m[key].(string)
	return s
}

func nestedStr(m map[string]any, objKey, field string) string {
	if m == nil {
		return ""
	}
	inner, ok := m[objKey].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := inner[field].(string)
	return s
}
