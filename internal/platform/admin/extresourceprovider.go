package admin

// extresourceprovider.go serves the MUTATIONS + cloud-test endpoints of the
// external-resource-providers surface (/api/v1/admin/external-resource-providers). The single
// read (GET list, ?externalServiceId) is ALREADY registered in handler.go
// (h.externalResourceProviders → admin.Repo.ListExternalResourceProviders) and is intentionally NOT
// re-registered here.
//
// Call graph:
//
//	create(body) = platformService.get(body.externalServiceId)  [→ ServiceNotFoundException if absent]
//	               THEN service.create(externalService, body):
//	                 hmacKey = hmacService.generate("Generated for external service provider <name> <url>")
//	                 save(ExternalResourceProvider{ auths:[{hmacKeyId}], url, externalServiceId, name })
//	               → single(saved)
//	update(id,body) = findById-or-404 → setId/Name/Url(body) → save → single(saved)
//	                  [externalServiceId + auths are PRESERVED from the stored doc]
//	delete(id)      = findById-or-404 → each auth → hmacService.delete(hmacKeyId) → repository.delete
//	                  [returns void → 200 with an EMPTY body]
//	test/billing-resources       = resolve provider/externalService/project/billingProfile then
//	                               providerService.getBillingResources(...) — a LIVE HTTP call to the
//	                               registered external billing provider (HMAC SigV4). [external integration point]
//	test/billing-resource-types  = providerService.getBillingResourceTypes(...) — same LIVE HTTP. [external integration point]
//
// HmacKey.generate is LOCAL crypto (md5/sha1 of a random UUID, the hmac_keys collection) — NOT a cloud
// call — so create persists the hmac key + the provider doc faithfully via the crud.go helpers.
// The two /test/** endpoints reach OUT to the external resource-provider's own HTTP API (the billing
// catalog) over an HMAC-signed request; that is an external action and is an external integration
// point: it returns 501 after no state change (the test endpoints are read-only probes that persist nothing).
//
// Perms (exact): create/update/delete gate ADMIN_SERVICE_MANAGE
// (admin:service:manage); the two /test reads gate ADMIN_SERVICE_READ (admin:service:read).
// Audit events are also written on create/delete (the hmac generate/delete are audited)
// — deferred this pass (// TODO(audit)).

import (
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/pkg/httpx"
)

const (
	erpManagePerm     = "admin:service:manage"
	erpReadPerm       = "admin:service:read"
	erpCollection     = "externalResourceProvider"
	erpHmacCollection = "hmac_keys"
	erpServiceColl    = "externalService"
)

// routeExtResourceProvider registers ONLY the external-resource-provider mutation + cloud-test
// routes. The {id} param name reuses the one handler.go already uses elsewhere (chi requires a
// single param name at a given path position; here the path prefix is distinct but we keep {id}
// for consistency).
func (h *Handler) routeExtResourceProvider(r chi.Router) {
	r.Post("/external-resource-providers", h.erpCreate)
	r.Put("/external-resource-providers/{id}", h.erpUpdate)
	r.Delete("/external-resource-providers/{id}", h.erpDelete)
	r.Post("/external-resource-providers/{id}/test/billing-resources", h.erpTestBillingResources)
	r.Post("/external-resource-providers/{id}/test/billing-resource-types", h.erpTestBillingResourceTypes)
}

// externalResourceProviderReq is the ExternalResourceProvider domain's mutable request-body
// fields. url + name are required (a blank value triggers the array-style validation
// envelope, not enforced here). externalServiceId is read
// only on create (update preserves the stored value).
type externalResourceProviderReq struct {
	ExternalServiceID string `json:"externalServiceId"`
	URL               string `json:"url"`
	Name              string `json:"name"`
}

// serviceNotFound is ServiceNotFoundException("Service not found: %s") — a plain runtime error
// (NOT an HttpError), so the default handler maps it to HTTP 500 with that message.
func serviceNotFound(id string) *httpx.HTTPError {
	return httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError, fmt.Sprintf("Service not found: %s", id))
}

// erpNotFound is notFound("External resource provider with id %s not found").
func erpNotFound(id string) *httpx.HTTPError {
	return httpx.NotFound(fmt.Sprintf("External resource provider with id %s not found", id))
}

// generateHmacKey generates a key: id = "pk"+md5hex(uuid), secretKey = "sk"+sha1hex(uuid),
// createdAt = now. Pure-local crypto (NOT a cloud call). Returns the stored JSON + the id.
// purpose:"provider" scopes this as an external-resource-provider credential: the Admin-API / MCP
// SigV4 lookup rejects purpose:"provider" keys, so a provider key can never authenticate as admin.
func generateHmacKey(description string) (pgdoc.M, string) {
	id := "pk" + hexMD5(randomUUID())
	secret := "sk" + hexSHA1(randomUUID())
	doc := pgdoc.M{
		"_id":         id,
		"secretKey":   secret,
		"createdAt":   time.Now().UTC(),
		"description": description,
		"purpose":     "provider",
	}
	return doc, id
}

func randomUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func hexMD5(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func hexSHA1(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

// erpCreate handles createExternalResourceProvider: resolve externalService (platformService.get) →
// 500 ServiceNotFoundException if absent → generate an HMAC key → persist the provider with
// auths=[{hmacKeyId}] → single(saved). ADMIN_SERVICE_MANAGE.
func (h *Handler) erpCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, erpManagePerm) {
		return
	}
	var req externalResourceProviderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// platformService.get(externalServiceId) → ServiceNotFoundException (500) when the service doc
	// is absent. The lookup is a plain datastore read (the decrypt step it also does has no visible
	// effect — we only need the doc's existence + its id).
	svc, err := h.repo.FindDoc(r.Context(), erpServiceColl, req.ExternalServiceID)
	if httpx.WriteError(w, err) {
		return
	}
	if svc == nil {
		httpx.WriteError(w, serviceNotFound(req.ExternalServiceID))
		return
	}
	externalServiceID := stringID(svc["_id"])

	// hmacService.generate(...) — local crypto, persisted to hmac_keys.
	hmacDoc, hmacKeyID := generateHmacKey(fmt.Sprintf("Generated for external service provider %s %s", req.Name, req.URL))
	if _, err := h.repo.InsertHmacKey(r.Context(), hmacDoc); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): hmacService.generate is audited (CREATE)

	// Build the stored provider: id is generated
	// by the datastore; only auths/url/externalServiceId/name are set (a null id is omitted).
	doc := pgdoc.M{
		"auths":             []pgdoc.M{{"hmacKeyId": hmacKeyID}},
		"url":               req.URL,
		"externalServiceId": externalServiceID,
		"name":              req.Name,
	}
	saved, err := h.repo.InsertDoc(r.Context(), erpCollection, doc)
	if httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(provider, CREATE, PLATFORM)
	httpx.OK(w, shapeDoc(saved))
}

// erpUpdate handles updateExternalResourceProvider: findById-or-404 → set id/name/url from the body
// (externalServiceId + auths PRESERVED) → save → single. ADMIN_SERVICE_MANAGE.
func (h *Handler) erpUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, erpManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req externalResourceProviderReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.FindDoc(r.Context(), erpCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, erpNotFound(id))
		return
	}
	before := maps.Clone(existing)
	// Only name + url are overwritten (setId(id) is a no-op on the persisted _id); externalServiceId
	// and auths are kept from the stored doc.
	existing["name"] = req.Name
	existing["url"] = req.URL
	if err := h.repo.ReplaceDoc(r.Context(), erpCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE audit: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.FindDoc(r.Context(), erpCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// erpDelete handles deleteExternalResourceProvider: findById-or-404 → delete each auth's hmac key →
// delete the provider. Returns void → a 200 with an EMPTY body (no envelope).
// ADMIN_SERVICE_MANAGE.
func (h *Handler) erpDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, erpManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.repo.FindDoc(r.Context(), erpCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, erpNotFound(id))
		return
	}
	// hmacService.delete(hmacKeyId) for each auth — a datastore delete on hmac_keys (delete is a silent
	// no-op if the key is absent, matching findById(...).ifPresent).
	for _, hmacKeyID := range erpAuthHmacKeyIDs(existing["auths"]) {
		if _, err := h.repo.DeleteHmacKey(r.Context(), hmacKeyID); httpx.WriteError(w, err) {
			return
		}
		// TODO(audit): hmacService.delete is auditAdmin(key, DELETE, PLATFORM)
	}
	if _, err := h.repo.DeleteDoc(r.Context(), erpCollection, id); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(provider, DELETE, PLATFORM)
	// the method returns void → 200 with an empty body (no envelope).
	w.WriteHeader(http.StatusOK)
}

// erpAuthHmacKeyIDs extracts the hmacKeyId of each Authorization in a stored doc's `auths` array
// (decoded from the store as []any of pgdoc.M). Nil-safe.
func erpAuthHmacKeyIDs(v any) []string {
	out := []string{}
	arr, ok := v.(pgdoc.A)
	if !ok {
		// the driver may also decode into []interface{}
		if a, ok2 := v.([]interface{}); ok2 {
			arr = pgdoc.A(a)
		} else {
			return out
		}
	}
	for _, e := range arr {
		m, ok := e.(pgdoc.M)
		if !ok {
			if pm, ok2 := e.(map[string]interface{}); ok2 {
				m = pgdoc.M(pm)
			} else {
				continue
			}
		}
		if id, ok := m["hmacKeyId"].(string); ok && id != "" {
			out = append(out, id)
		}
	}
	return out
}

// erpTestBillingResources handles testBillingResources: resolves the provider/externalService/project/
// billingProfile and then calls providerService.getBillingResources(...) — a LIVE HMAC-signed HTTP
// call to the registered external billing provider's catalog API. That external action is not wired:
// the provider must first be resolved (404 if the id is unknown — done before the call),
// then the cloud/external probe is NOT executed and we return 501. ADMIN_SERVICE_READ.
func (h *Handler) erpTestBillingResources(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, erpReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	provider, err := h.repo.FindDoc(r.Context(), erpCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if provider == nil {
		httpx.WriteError(w, erpNotFound(id))
		return
	}
	// External integration point: providerService.getBillingResources(billingContext, serviceContext) — fans out an
	// HMAC-signed POST to the external resource-provider's /billing_resources API. Purely external;
	// not wired this pass.
	httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
		"getBillingResources not implemented"))
}

// erpTestBillingResourceTypes handles testBillingResourceTypes: resolves the provider/externalService/
// project and then calls providerService.getBillingResourceTypes(...) — a LIVE HMAC-signed HTTP call
// to the external provider's /billing_resources/types API. Not wired (resolve-then-501). ADMIN_SERVICE_READ.
func (h *Handler) erpTestBillingResourceTypes(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, erpReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	provider, err := h.repo.FindDoc(r.Context(), erpCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if provider == nil {
		httpx.WriteError(w, erpNotFound(id))
		return
	}
	// External integration point: providerService.getBillingResourceTypes(serviceContext, true, null) — HMAC-signed POST to
	// the external provider's /billing_resources/types API. Purely external; not wired this pass.
	httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
		"getBillingResourceTypes not implemented"))
}

// stringID renders a stored _id (a hex string id) as the id the domain
// emits as externalServiceId. Nil/unknown → "".
func stringID(v any) string {
	switch id := v.(type) {
	case string:
		return id
	case fmt.Stringer:
		return id.String()
	default:
		if v == nil {
			return ""
		}
		return fmt.Sprintf("%v", v)
	}
}
