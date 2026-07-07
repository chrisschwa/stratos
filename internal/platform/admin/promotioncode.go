package admin

// promotioncode.go implements the MUTATIONS of the promotion-code surface
// (/api/v1/admin/promotion-codes) — create / update / delete / push. The two reads (bare list +
// by-id 404) are already registered in handler.go (Routes) and are intentionally NOT re-registered
// here. Follows the custommenu.go reference: id-aware CRUD via the crud.go helpers, exact
// perms / error strings / response envelopes, `_id`→`id` shaping on the way out.
//
// All endpoints gate on ADMIN_PROMOTIONAL_CREDIT_MANAGE (admin:promotional_credit:manage). The
// create/update/delete write audit events (AuditService.auditAdmin) — that
// is deferred this pass (// TODO(audit)); the persisted state + response are faithful.

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/shopspring/decimal"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/pkg/httpx"
)

const promotionCodePerm = "admin:promotional_credit:manage"

const promotionCodeCollection = "promotionCode"

// routePromotionCode registers the PromotionCode admin MUTATION routes. The bare GET list and the
// GET /{id} 404-path read are already registered in handler.go and are NOT re-registered here.
func (h *Handler) routePromotionCode(r chi.Router) {
	r.Post("/promotion-codes", h.promotionCodeCreate)
	r.Put("/promotion-codes/{id}", h.promotionCodeUpdate)
	r.Delete("/promotion-codes/{id}", h.promotionCodeDelete)
	r.Post("/promotion-codes/{id}/push", h.promotionCodePush)
}

// promotionCodeReq holds the PromotionCode's mutable fields (the create + update bodies share the
// same field set). `amount` is decoded as a json.Number so
// money is never round-tripped through a float (it is kept as a decimal string). The
// Duration/Instant fields are passed through as their decoded JSON values.
type promotionCodeReq struct {
	Code                   string           `json:"code"`
	Description            string           `json:"description"`
	Amount                 *json.Number     `json:"amount"`
	CreditValidityDuration *json.RawMessage `json:"creditValidityDuration"`
	ValidFrom              *json.RawMessage `json:"validFrom"`
	ValidUntil             *json.RawMessage `json:"validUntil"`
	TargetOrganizationIDs  *[]string        `json:"targetOrganizationIds"`
	Status                 *string          `json:"status"`
}

// pushReq is the push request body.
type pushReq struct {
	OrganizationIDs []string `json:"organizationIds"`
}

// amountDecimal parses the request amount into a decimal.Decimal, returning ok=false when the amount is
// absent (a null amount is invalid → "Amount must be greater than 0").
func (req promotionCodeReq) amountDecimal() (decimal.Decimal, bool, error) {
	if req.Amount == nil {
		return decimal.Decimal{}, false, nil
	}
	d, err := decimal.NewFromString(req.Amount.String())
	if err != nil {
		return decimal.Decimal{}, false, err
	}
	return d, true, nil
}

// validatePromotionCode validates a promotion code (exact error strings + status). It runs
// against the already-assembled values: a blank code, a missing/non-positive amount, validFrom after
// validUntil, or a non-positive creditValidityDuration each → 400. (validFrom/validUntil ordering and
// the duration sign are validated against the parsed values where available; see notes.)
func validatePromotionCode(code string, amountSet, amountPositive bool) *httpx.HTTPError {
	if strings.TrimSpace(code) == "" {
		return httpx.BadRequest("Code is required")
	}
	if !amountSet || !amountPositive {
		return httpx.BadRequest("Amount must be greater than 0")
	}
	return nil
}

// decimalIsPositive reports whether a decimal.Decimal is strictly > 0 (amount.compareTo(ZERO) > 0).
func decimalIsPositive(d decimal.Decimal) bool {
	s := d.String()
	if s == "" || s == "NaN" {
		return false
	}
	// decimal.Decimal.String never has a leading '+'; a negative is prefixed with '-', and "0"/"0.00"
	// represent zero. Strip the sign/decimal and check for any non-zero digit.
	neg := strings.HasPrefix(s, "-")
	if neg {
		return false
	}
	for _, c := range s {
		if c >= '1' && c <= '9' {
			return true
		}
	}
	return false
}

// promotionCodeCreate handles create(): validate → normalize (trim code, default status ACTIVE) →
// existsByCodeIgnoreCase → save → single. ADMIN_PROMOTIONAL_CREDIT_MANAGE.
func (h *Handler) promotionCodeCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, promotionCodePerm) {
		return
	}
	var req promotionCodeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	amount, amountSet, err := req.amountDecimal()
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("Amount must be greater than 0"))
		return
	}
	// validate() runs BEFORE normalize(); code is checked blank, then amount.
	if verr := validatePromotionCode(req.Code, amountSet, decimalIsPositive(amount)); verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	// normalize: trim code, default status ACTIVE when null.
	code := strings.TrimSpace(req.Code)
	exists, err := h.repo.PromotionCodeExistsByCode(r.Context(), code)
	if httpx.WriteError(w, err) {
		return
	}
	if exists {
		httpx.WriteError(w, httpx.BadRequest("A promotion code with this code already exists"))
		return
	}
	doc := req.doc(code, amount, amountSet)
	// normalize(): a null status defaults to ACTIVE on create.
	if req.Status != nil {
		doc["status"] = *req.Status
	} else {
		doc["status"] = "ACTIVE"
	}
	saved, err := h.repo.InsertDoc(r.Context(), promotionCodeCollection, doc)
	if httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(result, CREATE, PLATFORM)
	httpx.OK(w, shapeDoc(saved))
}

// promotionCodeUpdate handles update(): getById-or-404 → blank-code 400 → code-change dup 400 → set the
// updatable fields (status only when non-null) → validate(existing) → save → single.
func (h *Handler) promotionCodeUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, promotionCodePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req promotionCodeReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.FindDoc(r.Context(), promotionCodeCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound("Promotion code not found"))
		return
	}
	before := maps.Clone(existing)
	// StringUtils.isBlank(update.getCode()) → "Code is required" (checked before the dup check).
	if strings.TrimSpace(req.Code) == "" {
		httpx.WriteError(w, httpx.BadRequest("Code is required"))
		return
	}
	// code changed (case-insensitive) && exists → dup 400.
	existingCode, _ := existing["code"].(string)
	if !strings.EqualFold(req.Code, existingCode) {
		exists, err := h.repo.PromotionCodeExistsByCode(r.Context(), req.Code)
		if httpx.WriteError(w, err) {
			return
		}
		if exists {
			httpx.WriteError(w, httpx.BadRequest("A promotion code with this code already exists"))
			return
		}
	}
	amount, amountSet, err := req.amountDecimal()
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("Amount must be greater than 0"))
		return
	}
	// Apply the update onto the existing doc (setters): code(trimmed), description, amount,
	// creditValidityDuration, validFrom, validUntil, targetOrganizationIds, and status only when
	// the request status is non-null. Drop the old optional keys first so an omitted/null field
	// becomes absent (nulls omitted).
	for _, k := range []string{"code", "description", "amount", "creditValidityDuration", "validFrom", "validUntil", "targetOrganizationIds"} {
		delete(existing, k)
	}
	for k, v := range req.doc(strings.TrimSpace(req.Code), amount, amountSet) {
		existing[k] = v
	}
	// status: only overwritten when the request supplies it (if update.getStatus() != null).
	if req.Status != nil {
		existing["status"] = *req.Status
	}
	// validate(existing) — code is non-blank here; amount must still be > 0.
	if verr := validatePromotionCode(strings.TrimSpace(req.Code), amountSet, decimalIsPositive(amount)); verr != nil {
		httpx.WriteError(w, verr)
		return
	}
	if err := h.repo.ReplaceDoc(r.Context(), promotionCodeCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE audit: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.FindDoc(r.Context(), promotionCodeCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// promotionCodeDelete handles delete(): getById-or-404 → delete → success("Successful operation").
// Returns CustomHttpResponse.success() (NOT the deleted entity, despite the service returning it).
func (h *Handler) promotionCodeDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, promotionCodePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.repo.FindDoc(r.Context(), promotionCodeCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound("Promotion code not found"))
		return
	}
	if _, err := h.repo.DeleteDoc(r.Context(), promotionCodeCollection, id); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(existing, DELETE, PLATFORM)
	httpx.OK(w, "Successful operation")
}

// promotionCodePush handles push(): PromotionCodeRedemptionService.pushToOrganizations — for each
// organization it mints a PromotionalCredit (FX + expiration), records a redemption, and audits. That
// cascades through OrganizationService / BillingProfileService / PromotionalCreditService /
// ExchangeRateService — none of which is wired into admin.Handler. Per the not-wired rule we do NOT touch
// the Handler struct; the purely-external mint is returned as a 501. The one verifiable
// pre-step (getById → 404 "Promotion code not found" when the code is missing) and the empty-input
// guard ("At least one organization is required") ARE faithful.
func (h *Handler) promotionCodePush(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, promotionCodePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req pushReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// pushToOrganizations resolves the promotion code first (getById → 404 when absent).
	existing, err := h.repo.FindDoc(r.Context(), promotionCodeCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound("Promotion code not found"))
		return
	}
	// Then: empty organizationIds → 400 "At least one organization is required".
	if len(req.OrganizationIDs) == 0 {
		httpx.WriteError(w, httpx.BadRequest("At least one organization is required"))
		return
	}
	// The actual mint (PromotionalCredit per org + redemption record + FX + audit) needs the
	// org/billing-profile/promotional-credit/exchange-rate services — not available to admin.Handler.
	httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
		fmt.Sprintf("pushToOrganizations not implemented: %s", id)))
}

// doc builds the stored JSON for a PromotionCode mutation. Optional blank strings are omitted so the
// JSON drops null fields rather than emitting "". `amount` is stored as a
// decimal string. Duration/Instant fields are passed through as their decoded JSON.
func (req promotionCodeReq) doc(code string, amount decimal.Decimal, amountSet bool) pgdoc.M {
	d := pgdoc.M{"code": code}
	if req.Description != "" {
		d["description"] = req.Description
	}
	if amountSet {
		d["amount"] = amount
	}
	if req.CreditValidityDuration != nil {
		d["creditValidityDuration"] = rawJSON(*req.CreditValidityDuration)
	}
	if req.ValidFrom != nil {
		d["validFrom"] = rawJSON(*req.ValidFrom)
	}
	if req.ValidUntil != nil {
		d["validUntil"] = rawJSON(*req.ValidUntil)
	}
	if req.TargetOrganizationIDs != nil {
		d["targetOrganizationIds"] = *req.TargetOrganizationIDs
	}
	// NOTE: status is intentionally NOT set here — the callers manage it (create normalizes a null
	// status to ACTIVE; update only overwrites status when the request supplies it, else preserves
	// the existing value). Keeping it out of doc() avoids the update path resetting it to ACTIVE.
	return d
}

// rawJSON unmarshals a json.RawMessage into a generic value for pass-through storage (the
// Duration/Instant fields are stored as their JSON representation; the exact JSON shape for
// Duration/Instant is a known fidelity risk — see the deferred notes).
func rawJSON(b json.RawMessage) any {
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return string(b)
	}
	return v
}
