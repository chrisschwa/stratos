package admin

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// priceadjustmentrule.go implements the price-adjustment-rule surface
// (/api/v1/admin/price-adjustment-rules). None of these routes were
// previously registered in handler.go (the `/price-plan/**` admin routes are a DIFFERENT surface).
//
// Endpoints:
//   - create:               validate(name, pricePlanId) → save → single.   ADMIN_PRICE_PLAN_MANAGE
//   - get:                  findById → 404 "Price adjustment rule not found".ADMIN_PRICE_PLAN_READ
//   - update:               validate(name, pricePlanId) FIRST → get(404) →
//                           overwrite 5 fields (name/description/targets/tiers/enabled) → save.
//                                                                            ADMIN_PRICE_PLAN_MANAGE
//   - delete:               deleteById (NO 404 check) → success.            ADMIN_PRICE_PLAN_MANAGE
//   - getAllByPricePlanId:  findByPricePlanId → list.                       ADMIN_PRICE_PLAN_READ
//   - getRuleUsage:         get(404) → sum BillAdjustment.amount over OPEN bills carrying this rule
//                           → PriceAdjustmentRuleUsage. Not wired (money aggregation, see below).
//                                                                            ADMIN_PRICE_PLAN_READ
//
// The admin endpoints return the RAW PriceAdjustmentRule document (CustomHttpResponse.single /
// .list), NOT a client DTO. The raw JSON shape (id not _id, money startAmount/value as a JSON number
// backed by a decimal.Decimal stored as a decimal string in jsonb, nulls omitted, no _class) is produced by the typed priceAdjustmentRule
// domain in priceadjustmentrule_repo.go.
//
// create/update write audit events; delete has no audit. Deferred this
// pass (// TODO(audit)); state + response are faithful.
//
// ⚠ getRuleUsage is NOT WIRED: it sums BillAdjustment money (BigDecimal) across every OPEN bill whose
// adjustments reference this rule. That is a money aggregation over the bill collection + the
// golden-tested pricing/bill layer — not a simple persisted state read — so it returns 501 rather
// than emit a guessed/duplicated sum. The 404-when-the-rule-is-absent branch still runs first
// (faithful), so a missing rule returns the exact 404; an existing rule returns 501.

const (
	parManagePerm     = "admin:price_plan:manage"
	parReadPerm       = "admin:price_plan:read"
	parCollection     = "priceAdjustmentRule"
	parNotFoundMsg    = "Price adjustment rule not found"
	parNameRequired   = "Name must not be null"
	parPlanIDRequired = "Price plan ID must not be null"
)

// routePriceAdjustmentRule registers the PriceAdjustmentRuleController routes. chi allows only ONE
// param name at a given path position, so the two single-segment wildcard routes (`/{id}` and
// `/{id}/usage`) share the name `id`; the literal `price-plan` segment is a sibling and routes ahead
// of the wildcard with no conflict.
func (h *Handler) routePriceAdjustmentRule(r chi.Router) {
	r.Post("/price-adjustment-rules", h.priceAdjustmentRuleCreate)
	r.Get("/price-adjustment-rules/price-plan/{pricePlanId}", h.priceAdjustmentRulesByPricePlan)
	r.Get("/price-adjustment-rules/{id}/usage", h.priceAdjustmentRuleUsage)
	r.Get("/price-adjustment-rules/{id}", h.priceAdjustmentRuleGet)
	r.Put("/price-adjustment-rules/{id}", h.priceAdjustmentRuleUpdate)
	r.Delete("/price-adjustment-rules/{id}", h.priceAdjustmentRuleDelete)
}

// priceAdjustmentRuleReq is the price-adjustment-rule request body. Money inside tiers
// (startAmount) and modifiers (value) is decimal.Decimal so it stores as a decimal string in jsonb (pgdoc codec)
// and round-trips as a JSON number. pricePlanId is part of the create body (it is NOT a path var).
type priceAdjustmentRuleReq struct {
	Name        string                    `json:"name"`
	Enabled     bool                      `json:"enabled"`
	Description string                    `json:"description"`
	PricePlanID string                    `json:"pricePlanId"`
	Targets     []priceAdjustmentTarget   `json:"targets"`
	Tiers       []priceAdjustmentRuleTier `json:"tiers"`
}

// validate checks the rule: name required, then pricePlanId required. A failure
// maps to HTTP 400 {errors:{code:400,message:<msg>}} (== httpx.BadRequest). Name is checked first.
//
// NOTE: the underlying check is a NULL check, not a blank check; the JSON body always materializes a
// (possibly empty) String, so the only way to trip it from the wire is an absent/explicit-null field.
// A missing JSON key leaves the Go string "" — we treat "" as "null" here (the realistic FE inputs
// for these two required fields), matching the intent (a rule with no name / no plan is
// rejected 400 before persistence).
func (req priceAdjustmentRuleReq) validate() *httpx.HTTPError {
	if req.Name == "" {
		return httpx.BadRequest(parNameRequired)
	}
	if req.PricePlanID == "" {
		return httpx.BadRequest(parPlanIDRequired)
	}
	return nil
}

// toDomain builds the stored priceAdjustmentRule from the request (create path). Optional blank
// strings are left blank → omitted on the wire (`omitempty`).
func (req priceAdjustmentRuleReq) toDomain() priceAdjustmentRule {
	return priceAdjustmentRule{
		Name:        req.Name,
		Enabled:     req.Enabled,
		Description: req.Description,
		PricePlanID: req.PricePlanID,
		Targets:     req.Targets,
		Tiers:       req.Tiers,
	}
}

// priceAdjustmentRuleCreate handles create(): validate → save → single(saved). ADMIN_PRICE_PLAN_MANAGE.
func (h *Handler) priceAdjustmentRuleCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, parManagePerm) {
		return
	}
	var req priceAdjustmentRuleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	if verr := req.validate(); verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	saved, err := h.repo.InsertPriceAdjustmentRule(r.Context(), parCollection, req.toDomain())
	if httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): CREATE audit — auditService.auditAdmin(saved, CREATE, PLATFORM)
	httpx.OK(w, priceAdjustmentRuleToDto(saved))
}

// priceAdjustmentRuleGet handles get(): findById → single, or 404. ADMIN_PRICE_PLAN_READ.
func (h *Handler) priceAdjustmentRuleGet(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, parReadPerm) {
		return
	}
	rule, err := h.repo.PriceAdjustmentRuleByID(r.Context(), parCollection, chi.URLParam(r, "id"))
	if httpx.WriteError(w, err) {
		return
	}
	if rule == nil {
		httpx.WriteError(w, httpx.NotFound(parNotFoundMsg))
		return
	}
	httpx.OK(w, priceAdjustmentRuleToDto(rule))
}

// priceAdjustmentRuleUpdate handles update(): validate FIRST → get(404) → overwrite the 5 mutable
// fields (name/description/targets/tiers/enabled) → save → single. pricePlanId is NOT copied by the
// update (it is immutable after create), so the existing pricePlanId is preserved.
// ADMIN_PRICE_PLAN_MANAGE.
func (h *Handler) priceAdjustmentRuleUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, parManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req priceAdjustmentRuleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// The incoming rule is validated BEFORE the existing lookup → a 400 precedes the 404.
	if verr := req.validate(); verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	existing, err := h.repo.PriceAdjustmentRuleByID(r.Context(), parCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound(parNotFoundMsg))
		return
	}
	before, _ := h.repo.FindDoc(r.Context(), parCollection, id) // raw pre-mutation snapshot for the audit diff
	// Overwrite exactly the 5 fields the update copies (setName/setDescription/setTargets/
	// setTiers/setEnabled); id + pricePlanId are preserved.
	existing.Name = req.Name
	existing.Description = req.Description
	existing.Targets = req.Targets
	existing.Tiers = req.Tiers
	existing.Enabled = req.Enabled
	if err := h.repo.ReplacePriceAdjustmentRule(r.Context(), parCollection, id, *existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE audit: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.FindDoc(r.Context(), parCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, priceAdjustmentRuleToDto(existing))
}

// priceAdjustmentRuleDelete handles delete(): deleteById → success("Successful operation"). The
// delete calls deleteById with NO existence check (a missing id is a silent no-op → still 200).
// ADMIN_PRICE_PLAN_MANAGE.
func (h *Handler) priceAdjustmentRuleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, parManagePerm) {
		return
	}
	if _, err := h.repo.DeletePriceAdjustmentRule(r.Context(), parCollection, chi.URLParam(r, "id")); httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, "Successful operation")
}

// priceAdjustmentRulesByPricePlan handles getRulesByPricePlanId(): getAllByPricePlanId
// (findByPricePlanId, ALL — not only enabled) → list. ADMIN_PRICE_PLAN_READ.
func (h *Handler) priceAdjustmentRulesByPricePlan(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, parReadPerm) {
		return
	}
	rules, err := h.repo.PriceAdjustmentRulesByPricePlanID(r.Context(), parCollection, chi.URLParam(r, "pricePlanId"))
	if httpx.WriteError(w, err) {
		return
	}
	httpx.List(w, rules)
}

// priceAdjustmentRuleUsage handles getRuleUsage(): get(404) → Σ BillAdjustment.amount across every
// OPEN bill carrying an adjustment for this rule → PriceAdjustmentRuleUsage {ruleId, ruleName,
// openBillsCount, totalAdjustmentsAmount}. Pure datastore aggregation via the pgdoc decimal codec
// (money stored as a decimal string in jsonb, summed with decimal.Decimal arithmetic). ADMIN_PRICE_PLAN_READ.
func (h *Handler) priceAdjustmentRuleUsage(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, parReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	rule, err := h.repo.PriceAdjustmentRuleByID(r.Context(), parCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if rule == nil {
		// PriceAdjustmentRuleService.getRuleUsage calls get(ruleId) first → 404 when absent.
		httpx.WriteError(w, httpx.NotFound(parNotFoundMsg))
		return
	}
	count, total, err := h.repo.PriceAdjustmentRuleUsage(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, priceAdjustmentRuleUsageDto{
		RuleID: id, RuleName: rule.Name, OpenBillsCount: count,
		TotalAdjustmentsAmount: json.Number(total.String()),
	})
}
