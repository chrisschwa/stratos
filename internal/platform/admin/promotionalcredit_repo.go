package admin

// promotionalcredit_repo.go — domain-specific Repo method for the promotional-credit
// update path. The generic crud.go helpers (FindDoc / InsertDoc / SetFields / DeleteDoc) and
// admin.go's ListRawFiltered cover the other endpoints; only the (id AND billingProfileId) lookup
// (findByIdAndBillingProfileId) needs a bespoke method.

import (
	"context"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// PromotionalCreditByIDAndBillingProfile loads a credit by (id, billingProfileId)
// (findByIdAndBillingProfileId) — matches `_id` AND `billingProfileId`.
// Returns (nil,nil) when no document matches.
func (r *Repo) PromotionalCreditByIDAndBillingProfile(ctx context.Context, id, billingProfileID string) (pgdoc.M, error) {
	var doc pgdoc.M
	found, err := r.c(promotionalCreditColl).FindOne(ctx,
		pgdoc.M{"_id": id, "billingProfileId": billingProfileID}, &doc)
	if err != nil || !found {
		return nil, err
	}
	return doc, nil
}
