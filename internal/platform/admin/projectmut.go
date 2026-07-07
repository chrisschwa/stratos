package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/cloud/client"
	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/internal/platform/externalservice"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// projectmut.go implements the MUTATIONS (+ the two datastore-only reads) of the project surface
// (/api/v1/admin/project) that are not already registered in handler.go. The reads
// GET /project, /project/{id}, /project/by-user, /project/by-organization,
// /project/{billingProfileId}/billing-profile and /project/external-services/{externalServiceId}
// are ALREADY registered there and are intentionally NOT re-registered here.
//
// CLOUD LEGS: the endpoints that hit OpenStack run LIVE through h.projectCloud (ProjectCloudOps,
// wired in cmd/api — nil → they degrade to their original 501 responses, which is what the unit tests
// exercise):
//
//   - POST   /project                                  (create; optional bootstrapProject provision)
//   - POST   /project/{id}/sync                         (syncjob.SyncOne — whole-project / scoped)
//   - POST   /project/{id}/{status}  (ENABLED|DISABLED) (nova pause/unpause + status flip; resume async + sync)
//   - GET    /project/{id}/external-service/{esid}      (bootstrapProject onto the explicit service)
//   - GET    /project/unassociated-os-projects         (live keystone ListAllProjects, read-only)
//   - GET    /project/{id}/resources/counts            (cache aggregation — already live)
//
//   - DELETE /project/{id}      (scheduleProjectDeletion → CanDelete pre-check → flip SCHEDULED_FOR_DELETION)
//   - DELETE /project/{id}/now  (deleteProjectNow → flip DELETE_IN_PROGRESS → async Teardown cascade
//                                = project.Handler.TeardownProject: cloud resources + keystone tenant → DELETED)
//
// IN SCOPE (no cloud, pure datastore): GET /project/{id}/members, PUT /project/{id} (field-set),
// DELETE /project/{id}/cancel (status flip to ENABLED), and the no-cloud branches of
// POST /project/{id}/{status}.
//
// Audit: every mutation also writes an AuditService event — deferred (// TODO(audit)).

const projectCollection = "project"

// project perms (exact AdminPermissionEnum keys).
const (
	projectReadPerm   = "admin:project:read"
	projectCreatePerm = "admin:project:create"
	projectUpdatePerm = "admin:project:update"
	projectDeletePerm = "admin:project:delete"
)

// routeProjectMut registers ONLY the new ProjectAdmin mutation + missing-read routes. The {id}
// param name reuses the one handler.go already uses on /project/{id} (chi requires a single param
// name at a given path position). The static second segments (members / sync / now / cancel /
// external-service) take precedence over the {status} param route at the same position.
func (h *Handler) routeProjectMut(r chi.Router) {
	r.Post("/project", h.projectCreate)
	r.Get("/project/unassociated-os-projects", h.projectUnassociatedOsProjects)
	r.Get("/project/{id}/members", h.projectMembers)
	r.Get("/project/{id}/resources/counts", h.projectResourceCounts)
	r.Get("/project/{id}/external-service/{externalServiceId}", h.projectAddExternalService)
	r.Post("/project/{id}/sync", h.projectSync)
	r.Put("/project/{id}", h.projectUpdate)
	r.Delete("/project/{id}", h.projectScheduleDeletion)
	r.Delete("/project/{id}/now", h.projectDeleteNow)
	r.Delete("/project/{id}/cancel", h.projectCancelDeletion)
	r.Post("/project/{id}/{status}", h.projectUpdateStatus)
}

// projectIDNotFound is the exact 404 from ProjectAdminService.get(id) → translation
// PROJECT_ID_NOT_FOUND = "Project with id %s not found " (trailing space, interpolated). This is the
// message used by update / updateStatus / scheduleProjectForDeletion. (Note: getProject(id) used by
// the GET /{id} read uses a DIFFERENT message and is registered in handler.go.)
func projectIDNotFound(id string) *httpx.HTTPError {
	return httpx.NotFound(fmt.Sprintf("Project with id %s not found ", id))
}

// findProjectOr404 loads a project by id or writes the exact 404; returns (doc, ok).
func (h *Handler) findProjectOr404(w http.ResponseWriter, r *http.Request, id string) (pgdoc.M, bool) {
	doc, err := h.repo.FindDoc(r.Context(), projectCollection, id)
	if httpx.WriteError(w, err) {
		return nil, false
	}
	if doc == nil {
		httpx.WriteError(w, projectIDNotFound(id))
		return nil, false
	}
	return doc, true
}

// ── create ──────────────────────────────────────────────────────────────────────────────────────

// projectCreate handles create(): asserts →
// organization (404) + its billingProfileId (400) + userIds exist (404) → then the try{}:
// resolve the effective billing profile (validated but NOT stored on the project — the builder
// omits it), build the Project (ENABLED, org-OWNER membership), save, optionally bootstrap the
// external service (create-or-ADOPT the keystone tenant via projectCloud.Bootstrap), add the
// userIds as MEMBERs, save, audit CREATE. Any failure inside the try{} → catch →
// 500 "Error creating project".
func (h *Handler) projectCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectCreatePerm) {
		return
	}
	var req createProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// Assert.* → 400. projectName + organizationId are required.
	if req.ProjectName == "" {
		httpx.WriteError(w, httpx.BadRequest("Project name cannot be empty"))
		return
	}
	if req.OrganizationId == "" {
		httpx.WriteError(w, httpx.BadRequest("Organization ID cannot be empty"))
		return
	}
	if req.ExternalServiceId != "" && (h.projectCloud == nil || h.projectCloud.Bootstrap == nil) {
		// Provision requested but the cloud leg is unwired (tests / degraded boot) → 501, BEFORE
		// any persist so the create stays all-or-nothing.
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"project create (external-service provisioning) not implemented"))
		return
	}
	ctx := r.Context()
	org, err := h.repo.FindDoc(ctx, "organization", req.OrganizationId)
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	orgBpID, _ := org["billingProfileId"].(string)
	if orgBpID == "" {
		httpx.WriteError(w, httpx.BadRequest("Organization does not have a billing profile configured"))
		return
	}
	// Pre-try userIds existence check (getById is looped, 404 before anything persists).
	for _, uid := range req.UserIds {
		u, err := h.repo.FindDoc(ctx, "users", uid)
		if httpx.WriteError(w, err) {
			return
		}
		if u == nil {
			httpx.WriteError(w, httpx.NotFound("User not found: "+uid))
			return
		}
	}
	// ── try{} — any failure below maps to 500 "Error creating project" ──
	createFailed := func() {
		httpx.WriteError(w, httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError,
			"Error creating project"))
	}
	bpID := orgBpID
	if req.BillingProfileId != "" {
		bpID = req.BillingProfileId
	}
	bp, err := h.repo.FindDoc(ctx, "billingProfile", bpID)
	if err != nil || bp == nil {
		createFailed() // getBillingProfileById throws inside the try → caught → 500
		return
	}
	// Owner membership = the organization's OWNER member (400 when the org has none). Member docs
	// carry `roles: []string` (getRole() = roles[0]); the array-contains match is the datastore
	// equivalent — live-caught on the dev226 drill (`role` matched nothing → false "no owner").
	ownerDoc, err := h.repo.FindOneBy(ctx, "organization_members",
		pgdoc.M{"organizationId": req.OrganizationId, "roles": pgdoc.M{"$contains": "OWNER"}})
	if httpx.WriteError(w, err) {
		return
	}
	if ownerDoc == nil {
		httpx.WriteError(w, httpx.BadRequest("Organization has no owner"))
		return
	}
	ownerSub, _ := ownerDoc["sub"].(string)
	memberships := pgdoc.A{pgdoc.M{"sub": ownerSub, "role": "OWNER"}}
	// userIds join as MEMBER (skip anyone already a member, i.e. the owner).
	for _, uid := range req.UserIds {
		u, _ := h.repo.FindDoc(ctx, "users", uid)
		sub := ""
		if u != nil {
			if s, ok := u["sub"].(string); ok && s != "" {
				sub = s
			} else if s, ok := u["_id"].(string); ok {
				sub = s
			}
		}
		if sub == "" || sub == ownerSub {
			continue
		}
		dup := false
		for _, m := range memberships {
			if mm, ok := m.(pgdoc.M); ok && mm["sub"] == sub {
				dup = true
				break
			}
		}
		if !dup {
			memberships = append(memberships, pgdoc.M{"sub": sub, "role": "MEMBER"})
		}
	}
	doc := pgdoc.M{
		"name":           req.ProjectName,
		"organizationId": req.OrganizationId,
		"customInfo":     pgdoc.M{},
		"status":         "ENABLED",
		"memberships":    memberships,
		"services":       pgdoc.A{},
	}
	// InsertDoc assigns the id (a pgdoc hex string) and returns the doc carrying it as `_id`.
	created, err := h.repo.InsertDoc(ctx, projectCollection, doc)
	if err != nil {
		createFailed()
		return
	}
	pid, _ := created["_id"].(string)
	if req.ExternalServiceId != "" {
		// bootstrapProject with an explicit service (+ optional ADOPT of an existing keystone
		// project via externalProjectId — OpenstackProjectProvisionData).
		if err := h.projectCloud.Bootstrap(ctx, pid, req.ExternalServiceId, req.ExternalProjectId); err != nil {
			createFailed()
			return
		}
	}
	after, err := h.repo.FindDoc(ctx, projectCollection, pid)
	if err != nil || after == nil {
		createFailed()
		return
	}
	// CREATE PROJECT audit (middleware emits the admin event; snapshot = the created doc).
	audit.RecordSnapshots(ctx, nil, after)
	httpx.OK(w, shapeDoc(after))
}

// createProjectRequest is the create-project request body.
type createProjectRequest struct {
	ProjectName       string   `json:"projectName"`
	BillingProfileId  string   `json:"billingProfileId"`
	OrganizationId    string   `json:"organizationId"`
	ExternalServiceId string   `json:"externalServiceId"`
	ExternalProjectId string   `json:"externalProjectId"`
	UserIds           []string `json:"userIds"`
}

// ── reads (datastore-only / not wired) ──────────────────────────────────────────────────────────────────

// projectMembers handles listMembers: getProject(id) → for each membership, userAdminService
// .getUserBySub(sub) → list of User. Pure datastore (no cloud). Resolves the project first (404 via the
// getProject message — note the read GET /{id} uses getProject's message, which differs from the
// mutation message). Here projectService.getProjectById is called, whose message is the
// "The project with id %s was not found. " one already used by the registered GET /{id}.
func (h *Handler) projectMembers(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	proj, err := h.repo.FindDoc(r.Context(), projectCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if proj == nil {
		// getProject → projectService.getProjectById → PROJECT_ID_WAS_NOT_FOUND message.
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("The project with id %s was not found. ", id)))
		return
	}
	// Resolve each membership.sub → the User doc. Under greenfield the User lookup subsystem is the
	// users collection; we resolve the members directly from the users collection by sub so the list
	// matches the userAdminService.getUserBySub mapping. Missing users are skipped (a null user
	// would NPE, but greenfield projects have valid owner subs).
	subs := membershipSubs(proj)
	members := []pgdoc.M{}
	for _, sub := range subs {
		u, err := h.repo.FindOneBy(r.Context(), "users", pgdoc.M{"_id": sub})
		if httpx.WriteError(w, err) {
			return
		}
		if u == nil {
			u, err = h.repo.FindOneBy(r.Context(), "users", pgdoc.M{"sub": sub})
			if httpx.WriteError(w, err) {
				return
			}
		}
		if u != nil {
			members = append(members, shapeDoc(u))
		}
	}
	httpx.List(w, members)
}

// membershipSubs extracts the membership subs from a project doc (memberships:[{sub,role}]).
func membershipSubs(proj pgdoc.M) []string {
	out := []string{}
	raw, ok := proj["memberships"]
	if !ok {
		return out
	}
	arr, ok := raw.(pgdoc.A)
	if !ok {
		return out
	}
	for _, m := range arr {
		mm, ok := m.(pgdoc.M)
		if !ok {
			continue
		}
		if sub, ok := mm["sub"].(string); ok && sub != "" {
			out = append(out, sub)
		}
	}
	return out
}

// projectResourceCounts handles countProjectResources: cloudResourceCounter.countCloudResourcesByType(id).
// The counter is a pure datastore aggregation over the cloudResource CACHE (group by
// type+serviceId, SECURITY_GROUP minus the default sg, + TOTAL) — no live cloud call — so this
// is fully portable via cloud.Repo.CountByType.
func (h *Handler) projectResourceCounts(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectReadPerm) {
		return
	}
	counts, err := h.cloud.CountByType(r.Context(), chi.URLParam(r, "id"))
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, counts)
}

// projectUnassociatedOsProjects handles listUnassociatedOsProjects(?externalServiceId): resolve the
// external service, list ALL keystone projects (admin identity scope), subtract the ones already
// mapped to a stratos project via services[].externalProjectId, return the rest (read-only).
func (h *Handler) projectUnassociatedOsProjects(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectReadPerm) {
		return
	}
	esID := r.URL.Query().Get("externalServiceId")
	if esID == "" {
		// required query param absent → 400.
		httpx.WriteError(w, httpx.BadRequest("Required parameter 'externalServiceId' is not present."))
		return
	}
	es, ok := h.externalServiceOr404(w, r, esID)
	if !ok {
		return
	}
	if h.cloudNew == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"listUnassociatedOsProjects (keystone) not implemented"))
		return
	}
	cc, err := h.cloudClient(r.Context(), es, h.serviceRegions(es)[0])
	if httpx.WriteError(w, err) {
		return
	}
	osProjects, err := cc.ListAllProjects(r.Context())
	if httpx.WriteError(w, err) {
		return
	}
	// associated = every stratos project attached to this service → its externalProjectId.
	docs, err := h.repo.ListRawFiltered(r.Context(), projectCollection,
		pgdoc.M{"services": pgdoc.M{"$contains": pgdoc.M{"serviceId": esID}}})
	if httpx.WriteError(w, err) {
		return
	}
	associated := map[string]bool{}
	for _, d := range docs {
		svcs, _ := d["services"].(pgdoc.A)
		for _, s := range svcs {
			sm, ok := s.(pgdoc.M)
			if !ok {
				continue
			}
			if sm["serviceId"] == esID {
				if ext, _ := sm["externalProjectId"].(string); ext != "" {
					associated[ext] = true
				}
			}
		}
	}
	out := []client.KeystoneProject{}
	for _, op := range osProjects {
		if !associated[op.ID] {
			out = append(out, op)
		}
	}
	httpx.List(w, out)
}

// externalServiceOr404 resolves + decrypts an external service, or writes the exact
// platformExternalService.get error — the odd HTTP-400/code-404 "Cloud provider is not found.
// Please contact support." envelope (same as the serviceByID read).
func (h *Handler) externalServiceOr404(w http.ResponseWriter, r *http.Request, esID string) (*externalservice.ExternalService, bool) {
	if h.esSvc == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusBadRequest, http.StatusNotFound,
			"Cloud provider is not found. Please contact support."))
		return nil, false
	}
	es, err := h.esSvc.Get(r.Context(), esID)
	if httpx.WriteError(w, err) {
		return nil, false
	}
	if es == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusBadRequest, http.StatusNotFound,
			"Cloud provider is not found. Please contact support."))
		return nil, false
	}
	return es, true
}

// projectAddExternalService handles addExternalService (GET /{id}/external-service/{esid}): resolve
// the project (mutation 404) + external service (the 400/404 envelope), then
// projectService.bootstrapProject with the explicit service — create-or-reuse the keystone tenant
// and attach the ProjectExternalService entry. Bootstrap failure = the wrapped
// 500 "Cannot sync the project with the infrastructure. ".
func (h *Handler) projectAddExternalService(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	before, ok := h.findProjectOr404(w, r, id)
	if !ok {
		return
	}
	esID := chi.URLParam(r, "externalServiceId")
	if _, ok := h.externalServiceOr404(w, r, esID); !ok {
		return
	}
	if h.projectCloud == nil || h.projectCloud.Bootstrap == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"addExternalServiceToProject (bootstrap/provision) not implemented"))
		return
	}
	if err := h.projectCloud.Bootstrap(r.Context(), id, esID, ""); err != nil {
		// ProjectService.bootstrapProject catch → internalServerError(CANT_SYNC_PROJECT_WITH_INFRASTRUCTURE).
		httpx.WriteError(w, httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError,
			"Cannot sync the project with the infrastructure. "))
		return
	}
	after, err := h.repo.FindDoc(r.Context(), projectCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	// UPDATE PROJECT audit (adds withSnapshot(externalService); the middleware diff carries
	// the attached services entry).
	audit.RecordSnapshots(r.Context(), maps.Clone(before), after)
	httpx.OK(w, shapeDoc(after))
}

// ── sync (cloud integration point) ────────────────────────────────────────────────────────────────

// projectSync handles syncProject (POST /{id}/sync?serviceId): resolves the project (404 via
// getProjectById's message), then runs the live sync — whole-project (syncProjectLocked; gated on
// ENABLED) when serviceId is blank, else just that service (syncExternalService). Returns the
// project resolved BEFORE the sync.
func (h *Handler) projectSync(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	proj, err := h.repo.FindDoc(r.Context(), projectCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if proj == nil {
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("The project with id %s was not found. ", id)))
		return
	}
	if h.projectCloud == nil || h.projectCloud.Sync == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"syncProject (OpenStack sync) not implemented"))
		return
	}
	if err := h.projectCloud.Sync(r.Context(), id, r.URL.Query().Get("serviceId")); err != nil {
		httpx.WriteError(w, httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError, err.Error()))
		return
	}
	httpx.OK(w, shapeDoc(proj))
}

// ── update (datastore, in scope) ──────────────────────────────────────────────────────────────────────

// projectUpdate handles update (PUT /{id}): get(id)-or-404, then set
// name / billingProfileId / organizationId (all three set unconditionally, including to null
// when the field is absent), save, return single. Pure datastore. The three fields are overwritten:
// an absent field becomes null → omitted from the stored doc (nulls omitted).
func (h *Handler) projectUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req projectUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, ok := h.findProjectOr404(w, r, id)
	if !ok {
		return
	}
	before := maps.Clone(existing)
	// project.setName(req.name); setBillingProfileId(req.billingProfileId);
	// setOrganizationId(req.organizationId). Overwrite all three — drop old values first so an
	// omitted (null) field is cleared. A blank value persists as cleared (omitted on read).
	for _, k := range []string{"name", "billingProfileId", "organizationId"} {
		delete(existing, k)
	}
	if req.Name != "" {
		existing["name"] = req.Name
	}
	if req.BillingProfileId != "" {
		existing["billingProfileId"] = req.BillingProfileId
	}
	if req.OrganizationId != "" {
		existing["organizationId"] = req.OrganizationId
	}
	if err := h.repo.ReplaceDoc(r.Context(), projectCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE PROJECT: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.FindDoc(r.Context(), projectCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// projectUpdateReq is the project-update request body. `data` + `customInfo` are accepted by the
// DTO but update() never reads them (only name / billingProfileId / organizationId are applied).
type projectUpdateReq struct {
	Name             string `json:"name"`
	BillingProfileId string `json:"billingProfileId"`
	OrganizationId   string `json:"organizationId"`
}

// ── status (datastore flip + cloud suspend/resume integration point) ──────────────────────────────────

// projectUpdateStatus handles updateStatus (POST /{id}/{status}): get(id)-or-404, ProjectStatus
// .valueOf(status) (invalid → 500), 400 if already in the
// desired status, then:
//   - ENABLED / DISABLED → onProjectResume / onProjectSuspend against OpenStack (cloud, not wired): the
//     status is set only AFTER the cloud call succeeds, so we 501 without persisting.
//   - SCHEDULED_FOR_DELETION / DELETE_IN_PROGRESS → no cloud branch; it falls through and
//     saves the project with its status unchanged → pure datastore no-op save, return single.
func (h *Handler) projectUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	status := chi.URLParam(r, "status")
	if !isValidProjectStatus(status) {
		// ProjectStatus.valueOf(invalid) → 500.
		httpx.WriteError(w, httpx.NewError(http.StatusInternalServerError, http.StatusInternalServerError,
			fmt.Sprintf("Invalid project status %s", status)))
		return
	}
	existing, ok := h.findProjectOr404(w, r, id)
	if !ok {
		return
	}
	current, _ := existing["status"].(string)
	if current == status {
		// translation PROJECT_DESIRED_STATUS = "Project is already in desired status " (trailing space).
		httpx.WriteError(w, httpx.BadRequest("Project is already in desired status "))
		return
	}
	switch status {
	case "ENABLED", "DISABLED":
		if h.projectCloud == nil || h.projectCloud.PauseServers == nil {
			// Cloud leg unwired (tests / degraded boot) → 501; the status is set only after the
			// cloud call, so nothing persists.
			httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
				fmt.Sprintf("onProject%s not implemented", suspendResumeOp(status))))
			return
		}
		before := maps.Clone(existing)
		if status == "DISABLED" {
			// onProjectSuspend is SYNCHRONOUS: keystone member/API-user disable (no-op here —
			// the bootstrap creates no per-customer keystone users) + nova PAUSE of every cached
			// server (per-server errors swallowed inside), THEN the status flip persists.
			if err := h.projectCloud.PauseServers(r.Context(), id, true); httpx.WriteError(w, err) {
				return
			}
			if _, err := h.repo.SetFields(r.Context(), projectCollection, id, pgdoc.M{"status": status}); httpx.WriteError(w, err) {
				return
			}
		} else {
			// onProjectResume runs asynchronously: the datastore flip persists immediately; nova UNPAUSE + the
			// follow-up project sync (PlatformExternalService.onProjectResume also runs syncProject)
			// happen on a worker thread. Flip first so the async whole-project sync sees ENABLED.
			if _, err := h.repo.SetFields(r.Context(), projectCollection, id, pgdoc.M{"status": status}); httpx.WriteError(w, err) {
				return
			}
			ops := h.projectCloud
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
				defer cancel()
				_ = ops.PauseServers(ctx, id, false) // best-effort, swallows per-service errors
				if ops.Sync != nil {
					_ = ops.Sync(ctx, id, "")
				}
			}()
		}
		after, err := h.repo.FindDoc(r.Context(), projectCollection, id)
		if httpx.WriteError(w, err) {
			return
		}
		// UPDATE PROJECT audit with the status diff (resourceMetadata {status}).
		audit.RecordSnapshots(r.Context(), before, after)
		httpx.OK(w, shapeDoc(after))
		return
	default:
		// SCHEDULED_FOR_DELETION / DELETE_IN_PROGRESS: no cloud branch; save()s with the status
		// unchanged (the if/else never matches, so setStatus is never called). Faithful: save the doc
		// as-is and return it. (No status field change persists.)
		if err := h.repo.ReplaceDoc(r.Context(), projectCollection, id, existing); httpx.WriteError(w, err) {
			return
		}
		// TODO(audit): adminEventFromContext UPDATE PROJECT {status} SUCCESS.
		httpx.OK(w, shapeDoc(existing))
	}
}

func suspendResumeOp(status string) string {
	if status == "ENABLED" {
		return "Resume"
	}
	return "Suspend"
}

func isValidProjectStatus(s string) bool {
	switch s {
	case "ENABLED", "DISABLED", "SCHEDULED_FOR_DELETION", "DELETE_IN_PROGRESS":
		return true
	default:
		return false
	}
}

// ── deletion (datastore flips + cloud integration point) ───────────────────────────────────────────────

// projectScheduleDeletion handles delete (DELETE /{id}?cascade): get(id)-or-404 then
// projectService.scheduleProjectDeletion. scheduleProjectDeletion calls
// platformExternalService.canProjectBeDeleted (a CLOUD pre-check) BEFORE flipping the status to
// SCHEDULED_FOR_DELETION. Because the cloud pre-check gates the persisted flip, this is not wired: we do
// NOT flip the status (it wouldn't if canProjectBeDeleted threw). The already-scheduled fast path
// (status already SCHEDULED_FOR_DELETION / DELETE_IN_PROGRESS) returns the project WITHOUT touching
// the cloud — that branch is persisted-safe (no-op) and returns single.
func (h *Handler) projectScheduleDeletion(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectDeletePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, ok := h.findProjectOr404(w, r, id)
	if !ok {
		return
	}
	current, _ := existing["status"].(string)
	if current == "SCHEDULED_FOR_DELETION" || current == "DELETE_IN_PROGRESS" {
		// scheduleProjectDeletion no-ops (the guard skips the cloud check + the save) and returns
		// the project unchanged. Pure read-back, no cloud.
		// TODO(audit): adminEventFromContext DELETE PROJECT scheduled=true SUCCESS.
		httpx.OK(w, shapeDoc(existing))
		return
	}
	// platformExternalService.canProjectBeDeleted(ctx, cascade) — a live cloud pre-check gating the
	// flip. Unwired (tests / degraded boot) → the original 501, BEFORE any persist.
	if h.projectCloud == nil || h.projectCloud.CanDelete == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"scheduleProjectDeletion (canProjectBeDeleted cloud check) not implemented"))
		return
	}
	if err := h.projectCloud.CanDelete(r.Context(), id); httpx.WriteError(w, err) {
		return
	}
	// Pre-check passed → flip SCHEDULED_FOR_DELETION + stamp scheduledForDeletionAt.
	now := time.Now().UTC()
	if _, err := h.repo.SetFields(r.Context(), projectCollection, id, pgdoc.M{"status": "SCHEDULED_FOR_DELETION", "scheduledForDeletionAt": now}); httpx.WriteError(w, err) {
		return
	}
	existing["status"] = "SCHEDULED_FOR_DELETION"
	existing["scheduledForDeletionAt"] = now
	// TODO(audit): adminEventFromContext DELETE PROJECT scheduled=true SUCCESS.
	httpx.OK(w, shapeDoc(existing))
}

// projectDeleteNow handles deleteProjectNow (DELETE /{id}/now): get(id)-or-404, status→DELETE_IN_PROGRESS
// (persisted), then publishes "executeProjectDeletion" which runs the async OpenStack teardown. The
// status flip IS the faithful datastore effect and is applied; the publish → async cloud delete is
// not wired (return 501 after the flip).
func (h *Handler) projectDeleteNow(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectDeletePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, ok := h.findProjectOr404(w, r, id)
	if !ok {
		return
	}
	// executeProjectDeletion (async OpenStack teardown) is unwired (tests) → the original 501 BEFORE
	// the status flip (so a degraded boot never orphans a project in DELETE_IN_PROGRESS with no job).
	if h.projectCloud == nil || h.projectCloud.Teardown == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"executeProjectDeletion (async cloud teardown) not implemented"))
		return
	}
	// status=DELETE_IN_PROGRESS, persisted (the faithful effect of deleteProjectNow before the publish).
	if _, err := h.repo.SetFields(r.Context(), projectCollection, id, pgdoc.M{"status": "DELETE_IN_PROGRESS"}); httpx.WriteError(w, err) {
		return
	}
	existing["status"] = "DELETE_IN_PROGRESS"
	// Dispatch the async cloud cascade (fire-and-forget; deletes resources + tenant → marks DELETED).
	if err := h.projectCloud.Teardown(r.Context(), id); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): adminEventFromContext DELETE PROJECT scheduled=false SUCCESS.
	httpx.OK(w, shapeDoc(existing))
}

// projectCancelDeletion handles cancelProjectDeletion (DELETE /{id}/cancel): getProjectById(id)-or-404,
// 400 if DELETE_IN_PROGRESS ("Project is deleting. Cannot cancel deletion"), else status→ENABLED +
// clear scheduledForDeletionAt, save, return single. Pure datastore (no cloud). The 404 here uses
// getProjectById's message ("The project with id %s was not found. ").
func (h *Handler) projectCancelDeletion(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectDeletePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	existing, err := h.repo.FindDoc(r.Context(), projectCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("The project with id %s was not found. ", id)))
		return
	}
	current, _ := existing["status"].(string)
	if current == "DELETE_IN_PROGRESS" {
		httpx.WriteError(w, httpx.BadRequest("Project is deleting. Cannot cancel deletion"))
		return
	}
	// status=ENABLED, scheduledForDeletionAt=null (cleared). Persist via $set + $unset semantics:
	// SetFields sets status; clear the watermark by setting it to nil (omitted on read).
	if _, err := h.repo.SetFields(r.Context(), projectCollection, id, pgdoc.M{"status": "ENABLED", "scheduledForDeletionAt": nil}); httpx.WriteError(w, err) {
		return
	}
	// Re-read so the response reflects the persisted flip (returns the saved project).
	updated, err := h.repo.FindDoc(r.Context(), projectCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): adminEventFromContext UPDATE PROJECT deletionCancelled=true SUCCESS.
	httpx.OK(w, shapeDoc(updated))
}
