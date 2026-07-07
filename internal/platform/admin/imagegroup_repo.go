package admin

import (
	"context"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// imagegroup_repo.go holds the image-group-specific repo methods. The generic crud.go
// helpers cover findById / insert / delete; these add the `save()` upsert (used by the
// update endpoints, which key off the body/path id) and the findByCategoryId list (the
// getGroupsByCategory read + the deleteCategory cascade).

// imageUpsert is `save(entity)` for the update path: replace-by-id, inserting if the
// id does not yet exist (upsert). The doc carries only the domain fields (no id/_id) — the key is
// taken from the path id. This is a full replace, not a $set merge, so an omitted body field
// becomes null on the entity (dropped), matching save() of the request body.
func (r *Repo) imageUpsert(ctx context.Context, collection, id string, doc pgdoc.M) error {
	return r.c(collection).Upsert(ctx, id, doc)
}

// imageGroupsByCategory runs findByCategoryId → the raw ImageGroup docs (still
// carrying `_id`; the handler shapeDoc's each). Never nil.
func (r *Repo) imageGroupsByCategory(ctx context.Context, categoryID string) ([]pgdoc.M, error) {
	out := []pgdoc.M{}
	if err := r.c(imageGroupCollection).Find(ctx, pgdoc.M{"categoryId": categoryID}, &out); err != nil {
		return nil, err
	}
	return out, nil
}
