package providers

import (
	"context"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/cloud/client"
)

// image_sync.go adds the IMAGE read-sync provider: reconcile a
// project's glance images (incl. server snapshots — glance images with image_type=snapshot)
// so a vanished one is dropped from the cache and a drifted one refreshed.
//
// ⚠ LEAK GUARD (the dev125/187 leak class): glance list returns public/shared/community
// images from OTHER tenants too. Two-layer defence, listing with
// `owner=<externalProjectId>, limit=9999`:
//  1. the client ListImagesOwned passes `owner=<externalProjectId>`, AND
//  2. the pure mapper below post-filters `image.owner == externalProjectID`.
//
// externalProjectID == "" (an unscoped probe) disables the post-filter — never the syncjob
// path, which always scopes to the project's tenant.
//
// Data shape MIRRORS the create/notification/listImages shape ({"image": <imageToMap>},
// cloud_writes.go listImages / notification/fetcher.go) so an unchanged image produces no
// spurious "differs" under the keyed compare.

type ImageSyncProvider struct {
	cc                *client.Client
	region            string
	projectID         string
	externalProjectID string
}

func NewImageSyncProvider(cc *client.Client, region, projectID, externalProjectID string) *ImageSyncProvider {
	return &ImageSyncProvider{cc: cc, region: region, projectID: projectID, externalProjectID: externalProjectID}
}

func (p *ImageSyncProvider) Type() string      { return cloud.TypeImage }
func (p *ImageSyncProvider) ProjectID() string { return p.projectID }

// CompareKeys: isNeededToUpdate compares the "image" sub-key.
func (p *ImageSyncProvider) CompareKeys() []string { return []string{"image"} }

func (p *ImageSyncProvider) List(ctx context.Context) ([]cloud.CloudResource, error) {
	imgs, err := p.cc.ListImagesOwned(ctx, p.externalProjectID)
	if err != nil {
		return nil, err
	}
	return imagesToResources(imgs, p.region, p.projectID, p.externalProjectID), nil
}

// imagesToResources maps `data.image` maps to IMAGE CloudResources, post-filtering to the
// owning tenant (glance's owner field — layer 2 of the leak guard).
func imagesToResources(imgs []map[string]any, region, projectID, owner string) []cloud.CloudResource {
	out := make([]cloud.CloudResource, 0, len(imgs))
	for _, im := range imgs {
		id, _ := im["id"].(string)
		if id == "" {
			continue
		}
		if owner != "" {
			if o, _ := im["owner"].(string); o != owner {
				continue
			}
		}
		out = append(out, cloud.CloudResource{
			Type: cloud.TypeImage, ExternalID: id, Region: region, ProjectID: projectID,
			Data: map[string]any{"image": im},
		})
	}
	return out
}
