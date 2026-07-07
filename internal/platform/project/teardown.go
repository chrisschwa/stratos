package project

// teardown.go implements the project-deletion cloud cascade (the admin DELETE /project/{id}/now
// leg). It is scoped STRICTLY to the project's own cached resources + its own Keystone tenant(s) —
// it can never touch another project.

import (
	"context"
	"fmt"
	"sort"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
	"github.com/menlocloud/stratos/internal/cloud/providers"
)

// deletionOrder ranks a cloud-resource type for teardown: LOWER = delete FIRST. Dependents (that
// hold references onto others) go before their dependencies, so a best-effort sweep in this order
// minimizes "resource still in use" failures. Unknown/leaf types sort last.
func deletionOrder(t string) int {
	switch t {
	case cloud.TypeKubernetesCluster, cloud.TypeStack, cloud.TypeLoadBalancer:
		return 0 // composites that own many children
	case cloud.TypeTrilioRestore, cloud.TypeTrilioSnapshot, cloud.TypeTrilioWorkload, cloud.TypeTrilioBackupTarget:
		return 1
	case cloud.TypeIPSecSiteConnection:
		return 2
	case cloud.TypeVPNService, cloud.TypeVPNEndpointGroup, cloud.TypeIKEPolicy, cloud.TypeIPSecPolicy:
		return 3
	case cloud.TypeServer, cloud.TypeBaremetalServer:
		return 4 // deleting an instance releases its FIP/port/volume attachments
	case cloud.TypeFloatingIP:
		return 5
	case cloud.TypePort:
		return 6
	case cloud.TypeVolumeSnapshot, cloud.TypeShareSnapshot, cloud.TypeShareSnapshotGroup:
		return 7
	case cloud.TypeVolume, cloud.TypeVolumeBackup:
		return 8
	case cloud.TypeShare, cloud.TypeShareGroup:
		return 9
	case cloud.TypeShareNetwork, cloud.TypeShareSecurityService:
		return 10
	case cloud.TypeSubnet:
		return 11
	case cloud.TypeNetwork:
		return 12
	case cloud.TypeRouter:
		return 13 // after its subnets/ports are gone
	default:
		return 100 // security-group / keypair / image / bucket / dns-zone / secret / server-group / …
	}
}

// teardownSweeps is how many dependency-ordered passes the cascade makes: a resource that fails
// because a blocker still exists (e.g. a network with a lingering port) can succeed on a later pass
// once the blocker is deleted.
const teardownSweeps = 3

// TeardownProject cascade-deletes a project's cloud resources (best-effort, dependency-ordered, with
// a few retry sweeps), deletes the project's Keystone tenant(s), then marks the project DELETED. It
// runs to completion regardless of individual failures; the periodic sync/reconcile is the backstop
// for anything left behind. Returns a non-nil error summarising what could not be deleted (the
// project is still marked DELETED — a re-run or the sync job mops up the remainder).
func (h *Handler) TeardownProject(ctx context.Context, projectID string) error {
	p, err := h.svc.GetProjectByID(ctx, projectID)
	if err != nil {
		return err
	}
	resources, err := h.cloud.FindAllByProjectID(ctx, projectID)
	if err != nil {
		return err
	}
	sort.SliceStable(resources, func(i, j int) bool {
		return deletionOrder(resources[i].Type) < deletionOrder(resources[j].Type)
	})

	remaining := resources
	for sweep := 0; sweep < teardownSweeps && len(remaining) > 0; sweep++ {
		var stillLeft []cloud.CloudResource
		for i := range remaining {
			res := &remaining[i]
			cc, ok := h.tryTenantClient(ctx, p, res.ServiceID)
			if !ok {
				stillLeft = append(stillLeft, *res)
				continue
			}
			ws := providers.NewWriteService(cc, h.cloud)
			if err := ws.Delete(ctx, res.ServiceID, res.ExternalID); err != nil {
				stillLeft = append(stillLeft, *res)
			}
		}
		remaining = stillLeft
	}

	// Delete the Keystone tenant on each service the project is bootstrapped on (admin-scoped client,
	// not tenant-scoped — a tenant cannot delete itself). Best-effort.
	for _, svcID := range p.ServiceIDs() {
		extProj := p.ExternalProjectID(svcID)
		if extProj == "" {
			continue
		}
		es, err := h.esSvc.Get(ctx, svcID)
		if err != nil || es == nil {
			continue
		}
		adminCC, err := client.New(ctx, es.ClientConfig(h.cloudRegion))
		if err != nil {
			continue
		}
		_ = adminCC.DeleteProject(ctx, extProj)
	}

	// Terminal state: mark the project DELETED (keep the doc for audit history).
	p.Status = "DELETED"
	if err := h.svc.Save(ctx, p); err != nil {
		return err
	}
	if len(remaining) > 0 {
		return fmt.Errorf("teardown left %d resource(s) undeleted; the sync job will reconcile them", len(remaining))
	}
	return nil
}
