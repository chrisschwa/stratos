package admin

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/internal/platform/billing"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// savingscontract.go implements the savings-contract surface (/api/v1/admin/savings-contracts) —
// the mutations (POST create, PUT update, DELETE) + the two extra list reads (by-billing-profile,
// by-savings-plan-with-billing-profile). The two plain reads (bare list + GET /{id}) are already
// registered in handler.go (listRaw / rawByID) and are intentionally NOT re-registered here.
//
// Perms (AdminPermissionEnum → admin:* key): read endpoints gate ADMIN_SAVINGS_PLAN_READ
// (admin:savings_plan:read); the mutations gate ADMIN_SAVINGS_PLAN_MANAGE (admin:savings_plan:manage).
//
// update/delete also write audit events (auditService.auditAdmin) — deferred
// this pass (// TODO(audit)); the persisted state + the response envelope are faithful.
//
// Storage uses the id-aware crud.go helpers + shapeDoc (the endpoints return the RAW
// SavingsContract domain via CustomHttpResponse.single, exactly like the by-id read), with the
// money/tier business logic (create's discount-rate selection) decoded through the typed billing
// domain (decimal.Decimal) so the decimal compares are faithful.

const savingsContractPerm = "admin:savings_plan:read"
const savingsContractManagePerm = "admin:savings_plan:manage"

const savingsContractCollection = "savingsContract"

// routeSavingsContract registers ONLY the new savings-contract routes (the bare list and GET /{id}
// already live in handler.go). chi: the path position after `/savings-contracts/` already uses the
// param name `{id}` (the registered GET /{id}); the new {savingsContractId}/{billingProfileId}
// param routes MUST reuse `{id}` to avoid a chi registration panic. `billing-profile` and
// `savings-plan` are static siblings of the `{id}` param (no conflict).
func (h *Handler) routeSavingsContract(r chi.Router) {
	r.Post("/savings-contracts/{id}", h.savingsContractCreate)
	r.Put("/savings-contracts/{id}", h.savingsContractUpdate)
	r.Delete("/savings-contracts/{id}", h.savingsContractDelete)
	r.Get("/savings-contracts/billing-profile/{id}", h.savingsContractsByBillingProfile)
	r.Get("/savings-contracts/savings-plan/{id}", h.savingsContractsBySavingsPlan)
}

// createSavingsContractReq is the create-savings-contract request body (POST create).
// startDate is the enum CURRENT_MONTH | NEXT_MONTH.
type createSavingsContractReq struct {
	SavingsPlanID          string           `json:"savingsPlanId"`
	DurationMonths         int              `json:"durationMonths"`
	MonthlyCommittedAmount *decimal.Decimal `json:"monthlyCommittedAmount"`
	PaidUpfront            bool             `json:"paidUpfront"`
	StartDate              string           `json:"startDate"`
}

// savingsContractCreate handles createSavingsContract: validate billing profile + available plan +
// no-duplicate-active, compute start/end dates + discount rate from the matching schedule's tiers,
// persist a new ACTIVE contract → single(contract). Gated ADMIN_SAVINGS_PLAN_MANAGE.
func (h *Handler) savingsContractCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, savingsContractManagePerm) {
		return
	}
	billingProfileID := chi.URLParam(r, "id")
	var req createSavingsContractReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}

	// billingProfileService.getBillingProfileById(billingProfileId) — 404 if absent.
	bp, err := h.repo.savingsContractFindBillingProfile(r.Context(), billingProfileID)
	if httpx.WriteError(w, err) {
		return
	}
	if bp == nil {
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("Billing profile with id %s not found. ", billingProfileID)))
		return
	}

	// savingsPlanService.getAvailableSavingsPlan(savingsPlanId) = findByIdAndAvailable(id,true).
	plan, err := h.repo.savingsContractAvailablePlan(r.Context(), req.SavingsPlanID)
	if httpx.WriteError(w, err) {
		return
	}
	if plan == nil {
		httpx.WriteError(w, httpx.NotFound("Savings plan not found"))
		return
	}

	// existsBySavingsPlanIdAndBillingProfileIdAndStatus(planId, bpId, ACTIVE) → 400.
	exists, err := h.repo.savingsContractActiveExists(r.Context(), plan.ID, billingProfileID)
	if httpx.WriteError(w, err) {
		return
	}
	if exists {
		httpx.WriteError(w, httpx.BadRequest("You already have a savings contract for this savings plan"))
		return
	}

	startDate, perr := savingsContractStartDate(req.StartDate)
	if perr != nil {
		httpx.WriteError(w, perr)
		return
	}
	endDate := startDate.AddDate(0, req.DurationMonths, 0)

	// schedule lookup by durationMonths → 400 "No schedule found for the given duration".
	var schedule *billing.SavingsPlanSchedule
	for i := range plan.SavingSchedule {
		if plan.SavingSchedule[i].DurationMonths == req.DurationMonths {
			schedule = &plan.SavingSchedule[i]
			break
		}
	}
	if schedule == nil {
		httpx.WriteError(w, httpx.BadRequest("No schedule found for the given duration"))
		return
	}

	monthly := decimal.Zero
	if req.MonthlyCommittedAmount != nil {
		monthly = *req.MonthlyCommittedAmount
	}
	discountRate, derr := savingsContractDiscountRate(req.PaidUpfront, schedule, monthly)
	if derr != nil {
		httpx.WriteError(w, derr)
		return
	}

	// Build the stored doc (omit blank optional strings; targets passes through
	// the plan's targets — copies savingsPlan.getTargets()).
	doc := pgdoc.M{
		"billingProfileId": billingProfileID,
		"savingsPlanId":    plan.ID,
		"status":           billing.SavingsStatusActive,
		"durationMonths":   req.DurationMonths,
		"startDate":        startDate,
		"endDate":          endDate,
		"discountRate":     discountRate,
		"paidUpfront":      req.PaidUpfront,
	}
	if plan.Name != "" {
		doc["savingsPlanName"] = plan.Name
	}
	if plan.Targets != nil {
		doc["targets"] = plan.Targets
	}
	if req.MonthlyCommittedAmount != nil {
		doc["monthlyCommittedAmount"] = *req.MonthlyCommittedAmount
	}

	saved, err := h.repo.InsertDoc(r.Context(), savingsContractCollection, doc)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, shapeDoc(saved))
}

// savingsContractUpdate handles updateSavingsContract: findById-or-404 → overwrite the mutable fields
// from the request body → save → single. Gated ADMIN_SAVINGS_PLAN_MANAGE.
// Overwrites: billingProfileId, savingsPlanId, savingsPlanName, targets, startDate, endDate,
// discountRate, monthlyCommittedAmount, paidUpfront, orderId, status (durationMonths/createdAt are
// NOT overwritten by update). A null field in the body becomes null on the entity → dropped here.
func (h *Handler) savingsContractUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, savingsContractManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req savingsContractBody
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.FindDoc(r.Context(), savingsContractCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound("Savings contract not found"))
		return
	}
	before := maps.Clone(existing) // snapshot the pre-mutation state for the audit field-diff
	// Overwrite the mutable fields (drop-first so an omitted/null body field becomes null,
	// matching existing.setX(body.getX()) with body.getX()==null). durationMonths/createdAt
	// are NOT overwritten by update; paidUpfront is a primitive bool → always set from the
	// body (defaults false when omitted).
	for _, k := range []string{
		"billingProfileId", "savingsPlanId", "savingsPlanName", "targets", "startDate",
		"endDate", "discountRate", "monthlyCommittedAmount", "orderId", "status", "paidUpfront",
	} {
		delete(existing, k)
	}
	existing["paidUpfront"] = req.PaidUpfront
	for k, v := range req.setMap() {
		existing[k] = v
	}
	if err := h.repo.ReplaceDoc(r.Context(), savingsContractCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE audit: field-level before/after diff onto the audit event (the middleware
	// computes diffSnapshots(before, after) → AuditEvent.changes). Re-read the AFTER from the datastore so
	// both snapshots are store-decoded (same store types/shape) — avoids spurious diffs from comparing
	// the Go-rebuilt map against the datastore-decoded `before`.
	after, _ := h.repo.FindDoc(r.Context(), savingsContractCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// savingsContractDelete handles deleteSavingsContract: getById-or-404 → deleteById. Returns
// `void` → HTTP 200 with an EMPTY body (not the envelope,
// not 202). Gated ADMIN_SAVINGS_PLAN_MANAGE.
func (h *Handler) savingsContractDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, savingsContractManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.repo.FindDoc(r.Context(), savingsContractCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound("Savings contract not found"))
		return
	}
	if _, err := h.repo.DeleteDoc(r.Context(), savingsContractCollection, id); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditService.auditAdmin(contract, DELETE, ORGANIZATION)
	// The handler is `void` → HTTP 200, empty body.
	w.WriteHeader(http.StatusOK)
}

// savingsContractsByBillingProfile handles getSavingsContractsByBillingProfileId: the raw
// SavingsContract list for a billing profile (findByBillingProfileId) → list envelope.
// Gated ADMIN_SAVINGS_PLAN_READ.
func (h *Handler) savingsContractsByBillingProfile(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, savingsContractPerm) {
		return
	}
	items, err := h.repo.savingsContractsByBillingProfile(r.Context(), chi.URLParam(r, "id"))
	if httpx.WriteError(w, err) {
		return
	}
	for i := range items {
		shapeDoc(items[i])
	}
	httpx.List(w, items)
}

// savingsContractsBySavingsPlan handles getSavingsContractsBySavingsPlanId: the contracts for a
// savings plan, each enriched with its billingProfile (the $lookup aggregation), createdAt DESC →
// list envelope (List<SavingsContractDto>). Gated ADMIN_SAVINGS_PLAN_READ.
func (h *Handler) savingsContractsBySavingsPlan(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, savingsContractPerm) {
		return
	}
	items, err := h.repo.savingsContractsBySavingsPlanWithBillingProfile(r.Context(), chi.URLParam(r, "id"))
	if httpx.WriteError(w, err) {
		return
	}
	for i := range items {
		shapeDoc(items[i])
	}
	httpx.List(w, items)
}

// --- pure helpers (unit-tested) ---

// savingsContractStartDate computes the start date: CURRENT_MONTH → first day of the
// current month, NEXT_MONTH → first day of next month (both midnight UTC, per BillingUtils). Any
// other value is a bad enum (the switch is exhaustive over the two enum constants — a malformed
// body would 400 at binding; we mirror that with a 400).
func savingsContractStartDate(start string) (time.Time, *httpx.HTTPError) {
	now := time.Now().UTC()
	switch start {
	case "CURRENT_MONTH":
		return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC), nil
	case "NEXT_MONTH":
		first := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
		return first.AddDate(0, 1, 0), nil
	default:
		return time.Time{}, httpx.BadRequest("Invalid request body")
	}
}

// savingsContractDiscountRate computes getDiscountRate: from the schedule's upfront/no-upfront tiers,
// take the max discount among tiers whose startAmount <= monthlyCommittedAmount → 400 if none.
func savingsContractDiscountRate(paidUpfront bool, schedule *billing.SavingsPlanSchedule, monthly decimal.Decimal) (decimal.Decimal, *httpx.HTTPError) {
	tiers := schedule.NoUpfrontTiers
	if paidUpfront {
		tiers = schedule.UpfrontTiers
	}
	var best *decimal.Decimal
	for i := range tiers {
		t := tiers[i]
		if t.StartAmount == nil {
			continue
		}
		if t.StartAmount.Cmp(monthly) <= 0 {
			d := t.Discount
			if d == nil {
				z := decimal.Zero
				d = &z
			}
			if best == nil || d.Cmp(*best) > 0 {
				best = d
			}
		}
	}
	if best == nil {
		return decimal.Zero, httpx.BadRequest("No savings plan found for the given monthly commited amount")
	}
	return *best, nil
}

// savingsContractBody holds the mutable fields of the SavingsContract request body on PUT update.
// Money fields are *decimal.Decimal (stored as a decimal string in jsonb). A nil pointer /
// blank string → the field is omitted from the $set map (and dropped on the entity), matching
// setX(null).
type savingsContractBody struct {
	BillingProfileID       string            `json:"billingProfileId"`
	SavingsPlanID          string            `json:"savingsPlanId"`
	SavingsPlanName        string            `json:"savingsPlanName"`
	Targets                []json.RawMessage `json:"targets"`
	StartDate              *time.Time        `json:"startDate"`
	EndDate                *time.Time        `json:"endDate"`
	DiscountRate           *decimal.Decimal  `json:"discountRate"`
	MonthlyCommittedAmount *decimal.Decimal  `json:"monthlyCommittedAmount"`
	PaidUpfront            bool              `json:"paidUpfront"`
	OrderID                string            `json:"orderId"`
	Status                 string            `json:"status"`
}

// SavingsPlanTargetBody holds a SavingsPlanTarget on the update body (resourceType + filters).
type SavingsPlanTargetBody struct {
	ResourceType string                   `json:"resourceType"`
	Filters      []map[string]interface{} `json:"filters"`
}

// setMap builds the $set-equivalent JSON for the overwritten update fields. Blank strings and nil
// pointers are omitted (a null/omitted body field is dropped on the entity).
func (b savingsContractBody) setMap() pgdoc.M {
	d := pgdoc.M{}
	if b.BillingProfileID != "" {
		d["billingProfileId"] = b.BillingProfileID
	}
	if b.SavingsPlanID != "" {
		d["savingsPlanId"] = b.SavingsPlanID
	}
	if b.SavingsPlanName != "" {
		d["savingsPlanName"] = b.SavingsPlanName
	}
	if b.Targets != nil {
		targets := make([]SavingsPlanTargetBody, 0, len(b.Targets))
		for _, raw := range b.Targets {
			var t SavingsPlanTargetBody
			if err := json.Unmarshal(raw, &t); err == nil {
				if t.Filters == nil {
					t.Filters = []map[string]interface{}{}
				}
				targets = append(targets, t)
			}
		}
		d["targets"] = targets
	}
	if b.StartDate != nil {
		d["startDate"] = *b.StartDate
	}
	if b.EndDate != nil {
		d["endDate"] = *b.EndDate
	}
	if b.DiscountRate != nil {
		d["discountRate"] = *b.DiscountRate
	}
	if b.MonthlyCommittedAmount != nil {
		d["monthlyCommittedAmount"] = *b.MonthlyCommittedAmount
	}
	if b.OrderID != "" {
		d["orderId"] = b.OrderID
	}
	if b.Status != "" {
		d["status"] = b.Status
	}
	return d
}
