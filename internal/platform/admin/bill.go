package admin

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/internal/platform/billing"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// bill.go serves the /api/v1/admin/bill surface — the MUTATIONS (PUT update, DELETE) plus
// the reads still missing from handler.go.
//
// Already registered in handler.go and intentionally NOT re-registered here:
//   - GET /admin/bill                       (bare list → h.listRaw("admin:bill:read","bill"))
//
// Newly registered here:
//   - PUT    /admin/bill/{id}                       update — status/invoiceCurrency/invoiceGatewayId/billingProfileId
//   - DELETE /admin/bill/{id}                       delete
//   - GET    /admin/bill/{id}                       getBill → BillFinancialOverview (pricing recompute — not wired)
//   - GET    /admin/bill/{billingProfileId}/billing-profile  getBillsByBillingProfileId → overview page (not wired)
//   - GET    /admin/bill/download/{billId}          downloadStatement → PDF render (not wired)
//
// Perms (AdminPermissionEnum → admin:* key):
//   - read endpoints  → ADMIN_BILL_READ   = admin:bill:read
//   - update/delete   → ADMIN_BILL_MANAGE = admin:bill:manage
//
// update/delete write audit events (UPDATE/DELETE BILL) — deferred this
// pass (// TODO(audit)); the persisted state + the response envelope are faithful.
//
// ⚠ MONEY/PRICING: getBill / getBillFinancialOverviewPage build a BillFinancialOverview by RECOMPUTING
// net/gross/unpaid through the golden-tested pricing engine (getGrossAmountBill /
// getUnpaidGrossAmountBill / getNetAmountBillProductCurrencyWithAdjustments). That engine
// is not wired into the admin Handler (no new dep allowed this pass), and a raw passthrough would emit
// the wrong DTO shape + the wrong (un-taxed/un-FX'd) numbers — worse than failing. So both overview
// reads are NOT WIRED (501). downloadStatement renders a PDF — an external render,
// also not wired.

const billCollection = "bill"

const billReadPerm = "admin:bill:read"
const billManagePerm = "admin:bill:manage"

// routeBill registers the bill endpoints not already in handler.go. chi: the path
// position after `/bill/` already uses no param (the bare list is on `/bill`); the {id} param routes
// introduce `{id}`, and `download/{billId}` + `{billingProfileId}/billing-profile` must share that
// position carefully — `download` is a STATIC sibling of the `{id}` param (no conflict), while the
// `{billingProfileId}/billing-profile` two-segment route reuses the same first-segment param name
// `{id}` (chi requires one param name at a tree position) and is distinguished by its trailing
// `billing-profile` literal.
func (h *Handler) routeBill(r chi.Router) {
	r.Put("/bill/{id}", h.billUpdate)
	r.Delete("/bill/{id}", h.billDelete)
	r.Get("/bill/download/{billId}", h.billDownload)
	r.Get("/bill/{id}", h.billGet)
	r.Get("/bill/{id}/billing-profile", h.billsByBillingProfile)
}

// billNotFound is the exact 404 from getById:
// notFound(BILL_ID_NOT_FOUND, id). The `billIdNotFound`
// translation interpolates the id, with the interpolation convention used
// across the admin 404s.
func billNotFound(id string) *httpx.HTTPError {
	return httpx.NotFound(fmt.Sprintf("Bill with id %s not found", id))
}

// billUpdateReq is the mutable fields update copies from the request-body Bill:
// status, invoiceCurrency, invoiceGatewayId, billingProfileId. (Exactly these four are overwritten —
// every other Bill field, incl. items/createdAt, is left as the persisted entity's value.) A
// null/omitted body field becomes null on the entity → dropped here when empty.
type billUpdateReq struct {
	Status           string `json:"status"`
	InvoiceCurrency  string `json:"invoiceCurrency"`
	InvoiceGatewayID string `json:"invoiceGatewayId"`
	BillingProfileID string `json:"billingProfileId"`
}

// setMap builds the $set-equivalent JSON for the four overwritten fields. A blank string is omitted
// so the field is dropped on the entity (setX(null) → dropped from the JSON when empty).
func (req billUpdateReq) setMap() pgdoc.M {
	d := pgdoc.M{}
	if req.Status != "" {
		d["status"] = req.Status
	}
	if req.InvoiceCurrency != "" {
		d["invoiceCurrency"] = req.InvoiceCurrency
	}
	if req.InvoiceGatewayID != "" {
		d["invoiceGatewayId"] = req.InvoiceGatewayID
	}
	if req.BillingProfileID != "" {
		d["billingProfileId"] = req.BillingProfileID
	}
	return d
}

// billUpdate updates a bill: findById-or-404 → overwrite
// status/invoiceCurrency/invoiceGatewayId/billingProfileId from the body → save → single(bill).
// Gated ADMIN_BILL_MANAGE.
func (h *Handler) billUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, billManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req billUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.FindDoc(r.Context(), billCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, billNotFound(id))
		return
	}
	before := maps.Clone(existing) // snapshot the pre-mutation state for the audit field-diff
	// Overwrite exactly the four fields update() sets (drop-first so an omitted/null body
	// field becomes null, matching existing.setX(body.getX()) with body.getX()==null).
	for _, k := range []string{"status", "invoiceCurrency", "invoiceGatewayId", "billingProfileId"} {
		delete(existing, k)
	}
	for k, v := range req.setMap() {
		existing[k] = v
	}
	if err := h.repo.ReplaceDoc(r.Context(), billCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE audit: field-level before/after diff onto the audit event (the middleware
	// computes diffSnapshots(before, after) → AuditEvent.changes). Re-read the AFTER from the datastore so
	// both snapshots are store-decoded (same store types/shape) — avoids spurious diffs from comparing
	// the Go-rebuilt map against the datastore-decoded `before`.
	after, _ := h.repo.FindDoc(r.Context(), billCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// billDelete deletes a bill: deleteById (the bill is NOT
// looked up first — the audit event is logged then deleteById is called unconditionally) →
// success() = {data:"Successful operation"}. Gated ADMIN_BILL_MANAGE.
func (h *Handler) billDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, billManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	// deleteById is idempotent (no 404 on a missing id) — the handler returns
	// success regardless. DeleteDoc returns 0 deleted for a missing id; we ignore the count.
	if _, err := h.repo.DeleteDoc(r.Context(), billCollection, id); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): logAsync(adminEventFromContext DELETE BILL resourceId=id SUCCESS)
	httpx.OK(w, "Successful operation")
}

// billGet builds a BillFinancialOverview
// by recomputing net/gross/unpaid through the pricing engine. The engine is not wired into the
// admin Handler this pass and a raw passthrough would emit the wrong shape/numbers (see file header).
// Returns 404 first if the bill is absent (getBill → getById-or-404),
// otherwise 501. Gated ADMIN_BILL_READ.
func (h *Handler) billGet(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, billReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.repo.FindDoc(r.Context(), billCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, billNotFound(id))
		return
	}
	fo, err := h.billFinancialOverview(r.Context(), existing)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, fo)
}

// billsByBillingProfile maps each bill for a billing profile to a BillFinancialOverview.
// Same pricing recompute as billGet (see file header). Gated ADMIN_BILL_READ.
func (h *Handler) billsByBillingProfile(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, billReadPerm) {
		return
	}
	bpID := chi.URLParam(r, "id")
	bills, err := h.repo.ListRawFiltered(r.Context(), billCollection, pgdoc.M{"billingProfileId": bpID})
	if httpx.WriteError(w, err) {
		return
	}
	out := make([]pgdoc.M, 0, len(bills))
	for _, bd := range bills {
		fo, err := h.billFinancialOverview(r.Context(), bd)
		if httpx.WriteError(w, err) {
			return
		}
		out = append(out, fo)
	}
	httpx.List(w, out)
}

// billDownload renders the
// consumption-summary statement PDF and streams the
// bytes. Returns the bill 404 first (download → getById-or-404). Gated ADMIN_BILL_READ.
func (h *Handler) billDownload(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, billReadPerm) {
		return
	}
	id := chi.URLParam(r, "billId")
	bill, err := h.billing.BillByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if bill == nil {
		httpx.WriteError(w, billNotFound(id))
		return
	}
	// Statement-for billing profile (best-effort: an empty profile still renders a valid PDF).
	bp, _ := h.billing.FindByID(r.Context(), bill.BillingProfileID)
	if bp == nil {
		bp = &billing.BillingProfile{ID: bill.BillingProfileID}
	}
	data, filename, err := billing.BillStatementPDF(bill, bp)
	if httpx.WriteError(w, err) {
		return
	}
	w.Header().Set("Content-Type", "application/pdf")
	w.Header().Set("Content-Disposition", "attachment; filename="+filename)
	_, _ = w.Write(data)
}
