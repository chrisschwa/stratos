package admin

// instancemetadata_repo.go holds the one InstanceMetadataOption-specific repo method the generic
// crud.go helpers do not cover: the active-key-uniqueness check
// (existsByKeyAndEnabledTrue / existsByKeyAndEnabledTrueAndIdNot).

import (
	"context"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// InstanceMetadataKeyEnabledExists runs the two existence queries used by validateKeyUniqueness:
//   - excludeID == "" → existsByKeyAndEnabledTrue(key): any enabled doc with this exact key.
//   - excludeID != "" → existsByKeyAndEnabledTrueAndIdNot(key, id): enabled, this key, _id != id.
//
// The key match is exact (case-sensitive); ids are plain strings.
func (r *Repo) InstanceMetadataKeyEnabledExists(ctx context.Context, key, excludeID string) (bool, error) {
	filter := pgdoc.M{"key": key, "enabled": true}
	if excludeID != "" {
		filter["_id"] = pgdoc.M{"$ne": excludeID}
	}
	return r.c(instanceMetadataCollection).Exists(ctx, filter)
}
