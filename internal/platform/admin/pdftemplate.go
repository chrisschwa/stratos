package admin

import (
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"maps"

	"github.com/cbroglie/mustache"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// defaultPDFTemplates bundles the default assets (templates/invoice-template.html +
// statement-template.html) so revert-to-default is self-contained in the Go binary.
//
//go:embed templates/invoice-template.html templates/statement-template.html
var defaultPDFTemplates embed.FS

// pdftemplate.go implements the PDF-template surface (/api/v1/admin/pdf-templates) — the
// CRUD + by-type + placeholders + the render/preview endpoints. Follows the custommenu.go reference
// pattern: id-aware CRUD via the crud.go helpers, exact perms / error strings / response
// envelopes, `_id`→`id` shaping on the way out.
//
// READ perm  = admin:message_template:read  (ADMIN_MESSAGE_TEMPLATE_READ)
// WRITE perm = admin:message_template:manage (ADMIN_MESSAGE_TEMPLATE_MANAGE)
//
// NOTE: the bare list GET /pdf-templates is ALREADY registered in handler.go (listRaw); this file
// adds everything else (the mutations + the missing reads). The mutations call
// auditService-equivalent flows only implicitly — there is no audit call in this surface,
// so no // TODO(audit) is needed here.
//
// LIVE since dev232:
//   - POST /{id}/preview           → validateTemplateWithData (Mustache render → HTML string; a
//     render error returns the "Template validation error: …" STRING, 200)
//   - POST /{id}/revert-to-default → bundled default HTML (go:embed) + name/description reset + save
//
// NOT WIRED BY DESIGN (HTML→PDF needs a vendor renderer; there is no vendor-less HTML→PDF path — the
// bill/statement PDFs render natively via fpdf instead):
//   - POST /{id}/download      → generatePDFFromTemplate (application/pdf bytes)
//   - GET  /{id}/preview-pdf   → generatePDFFromTemplate (application/pdf bytes)
// These return 501. The datastore/state lookups they perform (getTemplateById → 404) ARE faithful and
// run first, so a bogus id still yields the exact 404.

const (
	pdfTemplateReadPerm   = "admin:message_template:read"
	pdfTemplateManagePerm = "admin:message_template:manage"
	pdfTemplateCollection = "pdfTemplate"
)

// routePDFTemplate registers the PDFTemplate admin routes that are NOT already in handler.go.
// (The bare list GET /pdf-templates stays on handler.go's listRaw.)
func (h *Handler) routePDFTemplate(r chi.Router) {
	r.Post("/pdf-templates", h.pdfTemplateCreate)
	// Static siblings BEFORE the {id} param routes (chi longest-prefix is fine, but keep order clear).
	r.Get("/pdf-templates/by-type/{type}", h.pdfTemplatesByType)
	r.Get("/pdf-templates/placeholders/{type}", h.pdfTemplatePlaceholders)
	r.Get("/pdf-templates/{id}", h.pdfTemplateGet)
	r.Put("/pdf-templates/{id}", h.pdfTemplateUpdate)
	r.Delete("/pdf-templates/{id}", h.pdfTemplateDelete)
	r.Post("/pdf-templates/{id}/download", h.pdfTemplateDownload)
	r.Post("/pdf-templates/{id}/revert-to-default", h.pdfTemplateRevert)
	r.Post("/pdf-templates/{id}/preview", h.pdfTemplatePreview)
	r.Get("/pdf-templates/{id}/preview-pdf", h.pdfTemplatePreviewPDF)
}

// pdfTemplateReq holds the PDF-template's mutable request-body fields. createdAt/updatedAt
// are server-managed and ignored on input.
type pdfTemplateReq struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Type        string `json:"type"`
}

// doc builds the stored JSON for a PDFTemplate. Optional strings are omitted when blank so the
// JSON drops null fields rather than emitting "". `type` is stored as the
// enum NAME ("INVOICE"/"STATEMENT"), so we keep the request value verbatim.
func (req pdfTemplateReq) doc() pgdoc.M {
	d := pgdoc.M{}
	if req.Name != "" {
		d["name"] = req.Name
	}
	if req.Description != "" {
		d["description"] = req.Description
	}
	if req.Content != "" {
		d["content"] = req.Content
	}
	if req.Type != "" {
		d["type"] = req.Type
	}
	return d
}

// parsePDFTemplateType parses the template type (upper-cased): only INVOICE/STATEMENT are
// valid. An unknown value → 500, carrying the message the default handler would surface.
func parsePDFTemplateType(raw string) (string, *httpx.HTTPError) {
	t := strings.ToUpper(raw)
	switch t {
	case "INVOICE", "STATEMENT":
		return t, nil
	default:
		return "", httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError,
			fmt.Sprintf("Invalid PDF template type %s", t))
	}
}

// pdfTemplateCreate handles createTemplate(): save → single(saved). ADMIN_MESSAGE_TEMPLATE_MANAGE.
func (h *Handler) pdfTemplateCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateManagePerm) {
		return
	}
	var req pdfTemplateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	saved, err := h.repo.InsertDoc(r.Context(), pdfTemplateCollection, req.doc())
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, shapeDoc(saved))
}

// pdfTemplateGet handles getTemplate(): getTemplateById-or-404 → single. ADMIN_MESSAGE_TEMPLATE_READ.
func (h *Handler) pdfTemplateGet(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateReadPerm) {
		return
	}
	doc, herr := h.pdfTemplateByID(r, chi.URLParam(r, "id"))
	if herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	httpx.OK(w, shapeDoc(doc))
}

// pdfTemplateUpdate handles updateTemplate(): getTemplateById-or-404 → validateTemplate (blank-content
// guard) → overwrite name/description/content/type → save → single. ADMIN_MESSAGE_TEMPLATE_MANAGE.
//
// validateTemplate ALSO renders a sample PDF (Mustache compile) to catch a
// malformed template; that render is not wired. The deterministic, faithful part — the
// blank-content 400 — IS covered; a structurally-bad-but-non-blank Mustache template that would be
// rejected is the deferred edge (noted in 'deferred'). The persisted state on the happy path matches.
func (h *Handler) pdfTemplateUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req pdfTemplateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, herr := h.pdfTemplateByID(r, id)
	if herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	// validateTemplate: blank content → 400 (HTMLBuiltInInvoiceClient.validateTemplate).
	if strings.TrimSpace(req.Content) == "" {
		httpx.WriteError(w, httpx.BadRequest("HTML content cannot be empty"))
		return
	}
	// Overwrite the 4 mutable fields (drop old first so an omitted/blank field becomes null).
	before := maps.Clone(existing)
	d := req.doc()
	for _, k := range []string{"name", "description", "content", "type"} {
		delete(existing, k)
	}
	for k, v := range d {
		existing[k] = v
	}
	if err := h.repo.ReplaceDoc(r.Context(), pdfTemplateCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE PDF_TEMPLATE: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.FindDoc(r.Context(), pdfTemplateCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// pdfTemplateDelete handles deleteTemplate(): getTemplateById-or-404 → deleteById → 202 (no body).
// Returns 202 with an empty body. ADMIN_MESSAGE_TEMPLATE_MANAGE.
func (h *Handler) pdfTemplateDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	if _, herr := h.pdfTemplateByID(r, id); herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	if _, err := h.repo.DeleteDoc(r.Context(), pdfTemplateCollection, id); httpx.WriteError(w, err) {
		return
	}
	httpx.Accepted(w)
}

// pdfTemplatesByType handles getTemplatesByType(): valueOf(type) → findByType → list envelope.
// ADMIN_MESSAGE_TEMPLATE_READ.
func (h *Handler) pdfTemplatesByType(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateReadPerm) {
		return
	}
	t, herr := parsePDFTemplateType(chi.URLParam(r, "type"))
	if herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	items, err := h.repo.ListRawFiltered(r.Context(), pdfTemplateCollection, pgdoc.M{"type": t})
	if httpx.WriteError(w, err) {
		return
	}
	for i := range items {
		shapeDoc(items[i])
	}
	httpx.List(w, items)
}

// pdfTemplatePlaceholders handles getPlaceholders(): valueOf(type) → the static placeholder map →
// single({...}). ADMIN_MESSAGE_TEMPLATE_READ.
func (h *Handler) pdfTemplatePlaceholders(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateReadPerm) {
		return
	}
	t, herr := parsePDFTemplateType(chi.URLParam(r, "type"))
	if herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	httpx.OK(w, pdfTemplatePlaceholdersByType(t))
}

// pdfTemplateRevert handles revertTemplateToDefault(): getTemplateById-or-404 → load the bundled
// default HTML for the template's TYPE + reset name/description → updateTemplate (save) → single.
// An unknown/absent type → the switch throws inside the try → 500 "Failed to revert template…".
// ADMIN_MESSAGE_TEMPLATE_MANAGE.
func (h *Handler) pdfTemplateRevert(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateManagePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, herr := h.pdfTemplateByID(r, id)
	if herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	t, _ := existing["type"].(string)
	var asset, name, desc string
	switch t {
	case "INVOICE":
		asset, name, desc = "templates/invoice-template.html", "Invoice Template", "Default template for invoices"
	case "STATEMENT":
		asset, name, desc = "templates/statement-template.html", "Statement Template", "Default template for statements"
	default:
		// template.getType() null/unknown → the switch throws → catch → 500 (message keeps
		// the raw "{}" placeholder — internalServerError(fmt, id, cause) never formats it).
		httpx.WriteError(w, httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError,
			"Failed to revert template id {} to default"))
		return
	}
	content, err := defaultPDFTemplates.ReadFile(asset)
	if httpx.WriteError(w, err) {
		return
	}
	before := maps.Clone(existing)
	existing["name"] = name
	existing["description"] = desc
	existing["content"] = string(content)
	if err := h.repo.ReplaceDoc(r.Context(), pdfTemplateCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	after, _ := h.repo.FindDoc(r.Context(), pdfTemplateCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// pdfTemplateDownload handles downloadTemplate(): getTemplateById-or-404 → generatePDFFromTemplate
// (req.template content + dummy data) → application/pdf bytes. The PDF render is not wired; the 404 path
// runs first and is faithful. ADMIN_MESSAGE_TEMPLATE_READ.
func (h *Handler) pdfTemplateDownload(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateReadPerm) {
		return
	}
	if _, herr := h.pdfTemplateByID(r, chi.URLParam(r, "id")); herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	// Drain/decode the body for shape-faithfulness (DownloadTemplateRequest{template}); unused (not wired).
	var req downloadTemplateRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
		"PDF download not implemented"))
}

// pdfTemplatePreview handles getTemplatePreview(): getTemplateById-or-404 (its TYPE picks the dummy
// data), Mustache-render the RAW request body (the request-body string content) with the dummy
// data → single(htmlString). A render failure returns the "Template validation error: …" STRING
// with HTTP 200 (validateTemplateWithData catches). ADMIN_MESSAGE_TEMPLATE_READ.
func (h *Handler) pdfTemplatePreview(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateReadPerm) {
		return
	}
	doc, herr := h.pdfTemplateByID(r, chi.URLParam(r, "id"))
	if herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	body, err := io.ReadAll(r.Body)
	if httpx.WriteError(w, err) {
		return
	}
	t, _ := doc["type"].(string)
	html, rerr := mustache.Render(string(body), pdfTemplateDummyData(t))
	if rerr != nil {
		html = "Template validation error: " + rerr.Error()
	}
	httpx.OK(w, html)
}

// pdfTemplatePreviewPDF handles getTemplatePdfPreview(): getTemplateById-or-404 →
// generatePDFFromTemplate → application/pdf bytes. The render is not wired; the 404 path runs first and
// is faithful. ADMIN_MESSAGE_TEMPLATE_READ.
func (h *Handler) pdfTemplatePreviewPDF(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, pdfTemplateReadPerm) {
		return
	}
	if _, herr := h.pdfTemplateByID(r, chi.URLParam(r, "id")); herr != nil {
		httpx.WriteError(w, herr)
		return
	}
	httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
		"PDF preview not implemented"))
}

// pdfTemplateByID loads a template by id (findById): the raw doc, or 404
// "PDF Template not found with id: <id>" (HttpError.notFound). Returns the doc still carrying `_id`
// (caller shapeDoc's before writing).
func (h *Handler) pdfTemplateByID(r *http.Request, id string) (pgdoc.M, *httpx.HTTPError) {
	doc, err := h.repo.FindDoc(r.Context(), pdfTemplateCollection, id)
	if err != nil {
		return nil, httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError, err.Error())
	}
	if doc == nil {
		return nil, httpx.NotFound("PDF Template not found with id: " + id)
	}
	return doc, nil
}

// downloadTemplateRequest is the download request body {template}.
type downloadTemplateRequest struct {
	Template pdfTemplateReq `json:"template"`
}

// pdfTemplateDummyData builds the dummy data — the sample values the
// preview renders with. Money values are the fixed STRINGS ("50.0"), so the preview HTML matches
// byte-for-byte. NOTE: the STATEMENT block has a bug — item2's period is written onto item1
// (item1.put twice, item2 never gets one) — kept so previews render identically.
func pdfTemplateDummyData(t string) map[string]any {
	switch t {
	case "INVOICE":
		now := time.Now()
		return map[string]any{
			"company":        map[string]any{"businessName": "ACME Corporation Ltd.", "vatId": "VAT12345678"},
			"companyAddress": map[string]any{"address": "1234 Business Avenue, Suite 100", "city": "New York", "country": "United States", "fullAddress": "1234 Business Avenue, Suite 100, New York, United States"},
			"invoice": map[string]any{
				"id": "inv-" + uuid.NewString()[:8], "series": "INV", "number": "2024-001", "currency": "USD",
				"dateOfIssue": now.Format("2006-01-02"), "dateOfDue": now.AddDate(0, 0, 30).Format("2006-01-02"),
				"amount": "85.00", "taxAmount": "17.00", "grossAmount": "102.00", "taxPercentage": "20",
			},
			"customer": map[string]any{"fullName": "John Smith", "email": "john.smith@example.com", "address": "456 Client Street, Customer City, CA 90210", "vatId": "CUST987654"},
			"items": []map[string]any{
				{"description": "Cloud Server Hosting", "details": "Premium server with 8GB RAM, 4 CPU cores", "qty": 1, "unitPrice": "50.0", "amount": "50.0", "unitPriceFormatted": "50.00 USD", "amountFormatted": "50.00 USD"},
				{"description": "SSL Certificate", "details": "Wildcard SSL certificate valid for 1 year", "qty": 1, "unitPrice": "25.0", "amount": "25.0", "unitPriceFormatted": "25.00 USD", "amountFormatted": "25.00 USD"},
				{"description": "Technical Support", "qty": 2, "unitPrice": "5.0", "amount": "10.0", "unitPriceFormatted": "5.00 USD", "amountFormatted": "10.00 USD"},
			},
			"item": map[string]any{"description": "Cloud Services Package", "qty": 1, "unitPrice": "85.0", "amount": "85.0", "unitPriceFormatted": "85.00 USD", "amountFormatted": "85.00 USD"},
		}
	case "STATEMENT":
		return map[string]any{
			"company":        map[string]any{"businessName": "ACME Corporation Ltd.", "vatId": "VAT12345678"},
			"companyAddress": map[string]any{"address": "1234 Business Avenue, Suite 100", "city": "New York", "country": "United States", "fullAddress": "1234 Business Avenue, Suite 100, New York, United States"},
			"customer":       map[string]any{"fullName": "John Smith", "email": "john.smith@example.com", "address": "456 Client Street, Customer City, CA 90210", "vatId": "CUST987654"},
			"items": []map[string]any{
				// item1 carries the period item2's write clobbered (the bug — see func comment).
				{"index": 1, "period": "Between 22.07.2025 09:38 and 31.07.2025 23:59", "name": "tms-1-prod-nwb-1", "resourceType": "instance", "amount": "50.00", "currency": "USD"},
				{"index": 2, "name": "tms-1-prod-nwb-2", "resourceType": "instance", "amount": "25.00", "currency": "USD"},
			},
			"statement":   map[string]any{"subtotal": "75.00", "currency": "USD", "total": "70.00", "startDate": "2025-07-22", "endDate": "2025-07-31"},
			"adjustments": []map[string]any{{"name": "10% Discount for Tiny Instances", "amount": "5.0"}},
			"payments":    []map[string]any{{"name": "From Account credits", "amount": "70.00"}},
		}
	default:
		return map[string]any{}
	}
}

// pdfTemplatePlaceholdersByType returns the static placeholder map for a type.
func pdfTemplatePlaceholdersByType(t string) map[string][]string {
	switch t {
	case "INVOICE":
		return map[string][]string{
			"invoice":        {"id", "number", "series", "currency", "dateOfIssue", "dateOfDue", "amount", "taxAmount", "grossAmount", "taxPercentage"},
			"company":        {"businessName", "vatId"},
			"companyAddress": {"country", "city", "address", "fullAddress"},
			"customer":       {"fullName", "email", "address", "vatId"},
			"items":          {"description", "qty", "unitPrice", "amount", "unitPriceFormatted", "amountFormatted"},
		}
	case "STATEMENT":
		return map[string][]string{
			"company":        {"businessName", "vatId"},
			"companyAddress": {"country", "city", "address", "fullAddress"},
			"customer":       {"fullName", "email", "address", "vatId"},
			"items":          {"index", "period", "name", "resourceType", "amount", "currency"},
			"statement":      {"subtotal", "currency", "total", "startDate", "endDate"},
			"adjustments":    {"name", "amount"},
			"payments":       {"name", "amount"},
		}
	default:
		return map[string][]string{}
	}
}
