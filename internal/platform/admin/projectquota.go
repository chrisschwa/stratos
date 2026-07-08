package admin

// projectquota.go — the per-project quota admin surface. PUT /admin/project/{id}/quota stores
// the quota config on the project doc (same stored-JSON posture as the provider default quota
// in externalservicemut.go — no OpenStack push). Shape: {"gpu": {"<model>": n, "*": n}} where
// model names use the shared GPU alias vocabulary (see internal/cloud/gpu.go). Enforcement is
// the project cloud gate (internal/platform/project/gpuquota.go), applied on server
// create/resize.

import (
	"encoding/json"
	"fmt"
	"maps"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// projectSetQuota handles PUT /project/{id}/quota: validate → store doc.quota = body →
// audit UPDATE (field-level diff) → the shaped doc. ADMIN_PROJECT_UPDATE.
func (h *Handler) projectSetQuota(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var body pgdoc.M
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	if err := validateProjectQuota(body); err != nil {
		httpx.WriteError(w, err)
		return
	}
	doc, ok := h.findProjectOr404(w, r, id)
	if !ok {
		return
	}
	before := maps.Clone(doc)
	if len(body) == 0 {
		delete(doc, "quota")
	} else {
		doc["quota"] = body
	}
	if err := h.repo.ReplaceDoc(r.Context(), projectCollection, id, doc); httpx.WriteError(w, err) {
		return
	}
	audit.RecordSnapshots(r.Context(), before, doc)
	httpx.OK(w, shapeDoc(doc))
}

// validateProjectQuota checks the quota shape: quota.gpu (when present) must be an object of
// non-negative integer limits keyed by GPU model alias (or "*").
func validateProjectQuota(body pgdoc.M) *httpx.HTTPError {
	gpuRaw, ok := body["gpu"]
	if !ok {
		return nil
	}
	gpu, ok := gpuRaw.(map[string]any)
	if !ok {
		return httpx.BadRequest("quota.gpu must be an object of {model: limit}")
	}
	for k, v := range gpu {
		f, ok := v.(float64) // JSON numbers decode to float64
		if !ok || f < 0 || f != float64(int64(f)) {
			return httpx.BadRequest(fmt.Sprintf("quota.gpu[%s] must be a non-negative integer", k))
		}
	}
	return nil
}
