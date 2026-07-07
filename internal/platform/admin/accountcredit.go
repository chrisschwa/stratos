package admin

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// accountcredit.go serves the AccountCredit admin CRUD (/api/v1/admin/account-credit):
// id-aware writes via the crud.go helpers, the exact perms /
// error strings / response envelopes, `_id`→`id` shaping on the way out.
//
// Perms (AdminPermissionEnum): the two READ endpoints gate ADMIN_ACCOUNT_CREDIT_READ
// ("admin:account_credit:read"); create/update/delete gate ADMIN_ACCOUNT_CREDIT_MANAGE
// ("admin:account_credit:manage").
//
// create/update/delete wrap an audit event — deferred this pass (// TODO(audit));
// the persisted state + the response are faithful.
//
// Currency conversion on create: create sets
// invoiceExchangeRate = getExchangeRate(baseCurrency, invoiceCurrency, now).
// getExchangeRate returns 1 when baseCurrency == invoiceCurrency, otherwise it calls
// the external ExchangeClient (BNR/Stratos HTTP) — not implemented. So create succeeds with no live call
// when the billing-configuration baseCurrency equals the billing profile currency (the greenfield
// single-currency case); a cross-currency create needs the FX lookup → returns 501 (see createAccountCredit).
//
// MONEY note: amounts are a decimal.Decimal stored as a decimal string in jsonb (the pgdoc decimal codec)
// and emitted via the raw store → shapeDoc passthrough, so on the wire they serialize the way encoding/json
// renders a decimal.Decimal (a quoted string) — a JSON string rather than a JSON number.
// That number-vs-string divergence is the known money-serialization gap (documented in the
// memory + the BillDto work) — flagged in 'deferred', not guessed at here. The state is faithful.

const (
	accountCreditReadPerm   = "admin:account_credit:read"
	accountCreditManagePerm = "admin:account_credit:manage"
	accountCreditCollection = "accountCredit"
)

// routeAccountCredit registers the account-credit routes. Base = /account-credit.
//
// chi gotcha: at the single-segment position under /account-credit/, the API uses {billingProfileId}
// (create) and {accountId} (update/delete) — but chi allows only ONE param name at a given tree
// position. We register it once as {id} and read it as the relevant value per handler. The 2-segment
// GET re-uses {id} for the first segment (it is the billingProfileId, which the GET handler
// actually ignores — it resolves purely by {accountId}).
func (h *Handler) routeAccountCredit(r chi.Router) {
	r.Get("/account-credit", h.accountCreditList)                 // ?billingProfileId=
	r.Post("/account-credit/{id}", h.accountCreditCreate)         // {billingProfileId}
	r.Put("/account-credit/{id}", h.accountCreditUpdate)          // {accountId}
	r.Delete("/account-credit/{id}", h.accountCreditDelete)       // {accountId}
	r.Get("/account-credit/{id}/{accountId}", h.accountCreditGet) // {billingProfileId}/{accountId}
}

// createAccountCreditReq is the create request body: { amount }.
type createAccountCreditReq struct {
	Amount json.Number `json:"amount"`
}

// updateAccountCreditReq is the update request body.
type updateAccountCreditReq struct {
	Currency        string      `json:"currency"`
	Amount          json.Number `json:"amount"`
	InvoiceCurrency string      `json:"invoiceCurrency"`
	InitialAmount   json.Number `json:"initialAmount"`
}

// accountCreditList lists credits: findAllByBillingProfile(billingProfileId) sorted
// createdAt DESC. NOTE: the list is wrapped in a single(...) envelope (NOT a paged list),
// so it is a bare {data:[...]} with NO paging — httpx.OK over the slice.
func (h *Handler) accountCreditList(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, accountCreditReadPerm) {
		return
	}
	bpID := r.URL.Query().Get("billingProfileId")
	docs, err := h.repo.AccountCreditsByBillingProfile(r.Context(), bpID)
	if httpx.WriteError(w, err) {
		return
	}
	for i := range docs {
		shapeDoc(docs[i])
	}
	httpx.OK(w, docs)
}

// accountCreditGet returns the credit: getById(accountId) → single, or 400 "Account credit not
// found" when absent. The {billingProfileId} path segment is ignored.
func (h *Handler) accountCreditGet(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, accountCreditReadPerm) {
		return
	}
	accountID := chi.URLParam(r, "accountId")
	doc, err := h.repo.FindDoc(r.Context(), accountCreditCollection, accountID)
	if httpx.WriteError(w, err) {
		return
	}
	if doc == nil {
		httpx.WriteError(w, httpx.BadRequest("Account credit not found"))
		return
	}
	httpx.OK(w, shapeDoc(doc))
}

// accountCreditCreate creates a credit for (billingProfileId, amount): resolve the billing profile
// (404 "Billing profile with id %s not found. " when absent), build the credit (amount/initialAmount=req.amount,
// currency=billingConfiguration.baseCurrency, invoiceCurrency=profile.currency, invoiceExchangeRate
// = FX(base, invoice)), save → single(saved). Cross-currency FX is an external lookup → 501.
func (h *Handler) accountCreditCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, accountCreditManagePerm) {
		return
	}
	billingProfileID := chi.URLParam(r, "id")
	var req createAccountCreditReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}

	// resolve the billing profile by id (404 when absent).
	currency, found, err := h.repo.BillingProfileCurrency(r.Context(), billingProfileID)
	if httpx.WriteError(w, err) {
		return
	}
	if !found {
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("Billing profile with id %s not found. ", billingProfileID)))
		return
	}

	// billingConfigurationService.getBillingConfiguration().getBaseCurrency().
	baseCurrency, err := h.repo.AccountCreditBaseCurrency(r.Context())
	if httpx.WriteError(w, err) {
		return
	}

	doc, err := h.repo.BuildAccountCreditDoc(req.Amount, baseCurrency, currency)
	if errors.Is(err, errAccountCreditFXSeam) {
		// Cross-currency FX would call the external ExchangeClient (external integration point).
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"account credit exchange-rate lookup not implemented"))
		return
	}
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// Tie the credit to the profile (set billingProfileId) — without it
	// the credit is orphaned: excluded from findAllByBillingProfile + the balance/account-credit total.
	doc["billingProfileId"] = billingProfileID

	saved, err := h.repo.InsertDoc(r.Context(), accountCreditCollection, doc)
	if httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditService.auditAdmin(result, CREATE, ORGANIZATION)
	httpx.OK(w, shapeDoc(saved))
}

// accountCreditUpdate updates a credit (id, uaReq): getById(id)-or-400 → overwrite invoiceCurrency,
// initialAmount, amount, currency (the 4 update-request fields) → save → single.
func (h *Handler) accountCreditUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, accountCreditManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req updateAccountCreditReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.FindDoc(r.Context(), accountCreditCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		// AccountCreditService.getById → HttpError.badRequest("Account credit not found")
		httpx.WriteError(w, httpx.BadRequest("Account credit not found"))
		return
	}

	before := maps.Clone(existing)
	set, err := h.repo.AccountCreditUpdateFields(req)
	if err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// The 4 fields are set directly (a null in the request becomes a null field). We
	// overwrite-or-drop the 4 mutable fields so an omitted/blank value clears, then apply the new.
	for _, k := range []string{"invoiceCurrency", "initialAmount", "amount", "currency"} {
		delete(existing, k)
	}
	for k, v := range set {
		existing[k] = v
	}
	if err := h.repo.ReplaceDoc(r.Context(), accountCreditCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE audit: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.FindDoc(r.Context(), accountCreditCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// accountCreditDelete deletes a credit: getById(id)-or-400 (the audit snapshot read), then
// delete (findById.ifPresent::delete) → success() → httpx.OK(w, "Successful operation").
func (h *Handler) accountCreditDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, accountCreditManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.repo.FindDoc(r.Context(), accountCreditCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		// AccountCreditAdminService.delete → accountCreditService.getById(id) (400) BEFORE delete.
		httpx.WriteError(w, httpx.BadRequest("Account credit not found"))
		return
	}
	if _, err := h.repo.DeleteDoc(r.Context(), accountCreditCollection, id); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditService.auditAdmin(existing, DELETE, ORGANIZATION)
	httpx.OK(w, "Successful operation")
}
