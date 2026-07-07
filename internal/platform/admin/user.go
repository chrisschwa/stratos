package admin

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/internal/platform/externalservice"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// user.go implements the admin user-management surface (/api/v1/admin/user).
//
// ⚠ The identity stack is an EMBEDDED authorization server over this same datastore
// users/userCredential store — there is NO Keycloak in it. create/delete are
// therefore plain datastore (+ the per-user keystone cleanup on delete); only impersonate (a local
// OAuth2 token MINT) stays unimplemented, because this service is a pure OIDC resource server
// (Keycloak owns token issuance).
//
// Call graph:
//
//	POST   /user                 create(AddUserRequest)   ADMIN_USER_CREATE        (datastore upsert-by-email + best-effort project invites)
//	GET    /user/{id}            getUser=getById          ADMIN_USER_READ          (datastore findById-or-404)
//	GET    /user                 listUsers                ADMIN_USER_READ          (ALREADY in handler.go — h.listRaw "users"; NOT re-registered)
//	GET    /user/sub/{sub}       getUserBySub             ADMIN_USER_READ          (datastore findBySub; null → {})
//	GET    /user/project/{pid}   getUserByProject         ADMIN_USER_READ          (project findById-or-404 → owner sub → user; null → {})
//	GET    /user/{id}/services   getUserExternalServices  ADMIN_USER_READ          (getById-or-404 → listByUser → DTO; empty greenfield → [])
//	PUT    /user/{id}            update(User)             ADMIN_USER_UPDATE        (datastore: sub/firstName/lastName/email overwrite)
//	DELETE /user/{id}            delete                   ADMIN_USER_DELETE        (onCleanUpUser keystone-user cleanup → deleteById)
//	POST   /user/{id}/impersonate impersonate            ADMIN_USER_IMPERSONATE   [not implemented: local OAuth2 token mint — divergent by design]
//
// Faithfulness: exact perms / error strings (USER_ID_NOT_FOUND = "User with id %s not found ",
// PROJECT_ID_WAS_NOT_FOUND = "The project with id %s was not found. ", USER_IS_IN_USE_FOR_PROJECTS =
// "User is in use for projects", CANNOT_DELETE_USER = "Cannot delete user " — all verbatim incl.
// trailing spaces). Datastore writes via the crud.go helpers + user_repo.go. Identity/Keycloak/token
// ops return 501. The service also writes audit events (deferred, // TODO(audit)).

const userCollection = "users"

const (
	userReadPerm        = "admin:user:read"
	userCreatePerm      = "admin:user:create"
	userUpdatePerm      = "admin:user:update"
	userDeletePerm      = "admin:user:delete"
	userImpersonatePerm = "admin:user:impersonate"
)

// routeUser registers the UserAdminController endpoints not already in handler.go. The {id} param
// name reuses the one chi already uses elsewhere at this path position. GET /user (the list) is
// already registered as h.listRaw("admin:user:read","users") and is intentionally NOT re-registered.
func (h *Handler) routeUser(r chi.Router) {
	r.Post("/user", h.userCreate)
	r.Get("/user/sub/{sub}", h.userBySub)
	r.Get("/user/project/{projectId}", h.userByProject)
	r.Get("/user/{id}/services", h.userExternalServices)
	r.Get("/user/{id}", h.userGet)
	r.Put("/user/{id}", h.userUpdate)
	r.Delete("/user/{id}", h.userDelete)
	r.Post("/user/{id}/impersonate", h.userImpersonate)
}

// userNotFound is the exact 404 (translation userIdNotFound, "User with id %s not found " —
// trailing space, interpolated). Used by getById-backed endpoints.
func userNotFound(id string) *httpx.HTTPError {
	return httpx.NotFound(fmt.Sprintf("User with id %s not found ", id))
}

// userUpdateReq holds the mutable fields the update() copies off the request-body User:
// sub, firstName, lastName, email (only these four are overwritten — see UserAdminService.update).
type userUpdateReq struct {
	Sub       string `json:"sub"`
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
}

// userGet handles getUser(id) = userService.getById: findById(_id)-or-404 → single(user).
func (h *Handler) userGet(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	u, err := h.repo.userByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		httpx.WriteError(w, userNotFound(id))
		return
	}
	httpx.OK(w, u)
}

// userBySub handles getUserBySub(sub) = userService.getBySub (findBySubOrderByCreatedAtDesc): a null
// user is wrapped in single(null) → empty {} envelope (the null data is dropped), NOT a 404.
func (h *Handler) userBySub(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userReadPerm) {
		return
	}
	u, err := h.repo.userBySub(r.Context(), chi.URLParam(r, "sub"))
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		httpx.Empty(w)
		return
	}
	httpx.OK(w, u)
}

// userByProject handles getUserByProject(projectId): projectAdminService.getProject =
// projectService.getProjectById (findById(_id)-or-404 "The project with id %s was not found. "),
// then userService.getUserBySub(project.getProjectOwner()) — owner sub from the OWNER membership
// (else the project's `owner` field). A null user → single(null) → {}.
func (h *Handler) userByProject(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userReadPerm) {
		return
	}
	pid := chi.URLParam(r, "projectId")
	proj, err := h.repo.projectByID(r.Context(), pid)
	if httpx.WriteError(w, err) {
		return
	}
	if proj == nil {
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("The project with id %s was not found. ", pid)))
		return
	}
	ownerSub := projectOwnerSub(proj)
	u, err := h.repo.userBySub(r.Context(), ownerSub)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		httpx.Empty(w)
		return
	}
	httpx.OK(w, u)
}

// userExternalServices handles getUserExternalServices(id): getById-or-404, then
// platformExternalService.listByUser(user) mapped to ExternalServiceDto. The user resolve (404 path)
// is the faithful behavior; the live OpenStack external-service listing is a cloud integration point
// (cloud-admin), so under greenfield the list is empty → {data:[], paging}. (Same stub posture
// as the existing /admin/service/{id}/user/services endpoint.)
func (h *Handler) userExternalServices(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	u, err := h.repo.userByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		httpx.WriteError(w, userNotFound(id))
		return
	}
	// External integration point: PlatformExternalService.listByUser → live OpenStack external services (cloud-admin).
	httpx.List(w, []any{})
}

// userUpdate handles update(id, User) = userService.update: getById-or-404 → overwrite
// sub/firstName/lastName/email off the request body → save → single(saved). Only those four fields
// are copied (the update copies exactly sub/firstName/lastName/email); all other persisted fields
// are preserved.
func (h *Handler) userUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req userUpdateReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.FindDoc(r.Context(), userCollection, id)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, userNotFound(id))
		return
	}
	before := maps.Clone(existing)
	// Overwrite the four mutable fields. The setters assign unconditionally (a null/blank request
	// value would null the field); we mirror that — set the value, and drop the key when blank so the
	// stored doc / null-omitting JSON matches a null assignment.
	for k, v := range map[string]string{"sub": req.Sub, "firstName": req.FirstName, "lastName": req.LastName, "email": req.Email} {
		if v == "" {
			delete(existing, k)
		} else {
			existing[k] = v
		}
	}
	if err := h.repo.ReplaceDoc(r.Context(), userCollection, id, existing); httpx.WriteError(w, err) {
		return
	}
	// UPDATE USER: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.FindDoc(r.Context(), userCollection, id)
	audit.RecordSnapshots(r.Context(), before, after)
	httpx.OK(w, shapeDoc(existing))
}

// userDelete handles delete(id): getUser(id)-or-404, then if projectAdminService.isUserInUse(sub)
// → 400 "User is in use for projects"; else UserAdminService.delete = onCleanUpUser (delete the
// user's per-service keystone users — OpenstackUserService.deleteOpenStackUser per NON-SHARED
// openstack service in user.services[], via config.openstackUserId) → deleteById + audit; a failed
// keystone delete → 400 "Cannot delete user ". Greenfield users carry no services[] (the
// bootstrap creates no per-customer keystone users) → allMatch over the empty set = true → plain
// Datastore delete.
func (h *Handler) userDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userDeletePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	u, err := h.repo.userByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		httpx.WriteError(w, userNotFound(id))
		return
	}
	inUse, err := h.repo.userInUse(r.Context(), u.Sub)
	if httpx.WriteError(w, err) {
		return
	}
	if inUse {
		httpx.WriteError(w, httpx.BadRequest("User is in use for projects"))
		return
	}
	if !h.cleanUpUser(w, r, id) {
		return
	}
	if err := h.repo.deleteUserByID(r.Context(), id); httpx.WriteError(w, err) {
		return
	}
	// DELETE USER audit (auditAdmin PLATFORM — the middleware emits the admin event).
	httpx.OK(w, "Successful operation")
}

// cleanUpUser handles onCleanUpUser: for each of the user's attached
// NON-SHARED openstack services, keystone-delete config.openstackUserId; allMatch gates the datastore
// delete. Writes the response (400 "Cannot delete user " / 501 when a cleanup is needed but the
// cloud factory is unwired) and returns false when the caller must stop.
func (h *Handler) cleanUpUser(w http.ResponseWriter, r *http.Request, userID string) bool {
	raw, err := h.repo.FindDoc(r.Context(), "users", userID)
	if httpx.WriteError(w, err) {
		return false
	}
	svcs, _ := raw["services"].(pgdoc.A)
	if len(svcs) == 0 {
		return true // no per-user cloud identities → nothing to clean (the greenfield case)
	}
	if h.esSvc == nil || h.cloudNew == nil {
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"onCleanUpUser not implemented"))
		return false
	}
	ok := true
	for _, s := range svcs {
		sm, isM := s.(pgdoc.M)
		if !isM {
			continue
		}
		esID, _ := sm["serviceId"].(string)
		osUserID := ""
		if cfgm, isM := sm["config"].(pgdoc.M); isM {
			osUserID, _ = cfgm["openstackUserId"].(string)
		}
		if esID == "" || osUserID == "" {
			continue
		}
		es, err := h.esSvc.Get(r.Context(), esID)
		if err != nil || es == nil || es.Type != externalservice.TypeCloud ||
			es.Provider() != "openstack" || es.Shared() {
			continue // filters isOpenStack + isNotShared
		}
		cc, cerr := h.cloudClient(r.Context(), es, h.serviceRegions(es)[0])
		if cerr != nil || cc == nil {
			ok = false
			continue
		}
		if derr := cc.DeleteUser(r.Context(), osUserID); derr != nil {
			slog.Error("onCleanUpUser: keystone delete", "user", userID, "openstackUserId", osUserID, "err", derr)
			ok = false
		}
	}
	if !ok {
		httpx.WriteError(w, httpx.BadRequest("Cannot delete user "))
		return false
	}
	return true
}

// userCreate handles create(AddUserRequest): findFirstByEmail → 400 "User with this email already
// exists" if taken; else UserService.create — which is a plain datastore upsert-by-email (sub = random
// UUID, NO password/credential, NO identity-provider call: the identity stack is the embedded
// auth server over this same `users` collection) — then best-effort
// projectInviteService.inviteToProject per projectId (each caught + logged). Returns the
// created User (findBySubOrderByCreatedAtDesc).
func (h *Handler) userCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userCreatePerm) {
		return
	}
	var req struct {
		FirstName  string   `json:"firstName"`
		LastName   string   `json:"lastName"`
		Email      string   `json:"email"`
		ProjectIDs []string `json:"projectIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.userByEmail(r.Context(), req.Email)
	if httpx.WriteError(w, err) {
		return
	}
	if existing != nil {
		httpx.WriteError(w, httpx.BadRequest("User with this email already exists"))
		return
	}
	now := time.Now().UTC()
	doc := pgdoc.M{
		"sub":          uuid.NewString(),
		"email":        req.Email,
		"createdAt":    now,
		"modelVersion": 1,
		"customInfo":   pgdoc.M{},
		"consent":      pgdoc.A{},
		"identities":   pgdoc.A{},
		"services":     pgdoc.A{},
		"metadata":     pgdoc.M{},
	}
	if req.FirstName != "" {
		doc["firstName"] = req.FirstName
	}
	if req.LastName != "" {
		doc["lastName"] = req.LastName
	}
	if _, err := h.repo.insertUserDoc(r.Context(), doc); httpx.WriteError(w, err) {
		return
	}
	created, err := h.repo.userBySub(r.Context(), doc["sub"].(string))
	if httpx.WriteError(w, err) {
		return
	}
	// Optional invites — best-effort (each caught + logged per projectId).
	if h.inviteToProject != nil {
		for _, pid := range req.ProjectIDs {
			if err := h.inviteToProject(r.Context(), created, req.Email, pid); err != nil {
				slog.Error("admin user-create: invite", "user", created.ID, "project", pid, "err", err)
			}
		}
	}
	// CREATE USER audit (the middleware emits the admin event).
	httpx.OK(w, created)
}

// userImpersonate handles impersonate(id): ImpersonationService.impersonate — getUser(id)-or-404
// "User not found", then the embedded authorization server MINTS local OAuth2 tokens
// (RegisteredClient "cloud-dashboard", grant JWT_BEARER) and returns
// {"url": "<ui>/login#access_token=…&id_token=…"}. DIVERGENT BY DESIGN: token
// issuance belongs to Keycloak and this service is a pure OIDC resource server — it cannot mint
// tokens the UI would accept. Stays 501 after the faithful user-exists check. (A Keycloak
// token-exchange/impersonation leg would be a NEW feature — do only on request.)
func (h *Handler) userImpersonate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userImpersonatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	u, err := h.repo.userByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		// ImpersonationService throws notFound("User not found") when the target user is absent.
		httpx.WriteError(w, httpx.NotFound("User not found"))
		return
	}
	// Not implemented: token generation / OAuth2 authorization (cloud-dashboard client) → ImpersonationResult
	// {redirectUrl, accessToken, idToken}; the controller returns single(Map.of("url", redirectUrl)).
	// The Go service issues no tokens (pure OIDC resource server). TODO(audit): IMPERSONATE event.
	httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
		"impersonation token generation not implemented"))
}

// projectOwnerSub returns the project owner sub: the OWNER membership's sub, else the
// project's `owner` field. memberships is an array of {sub, role} sub-docs.
func projectOwnerSub(proj pgdoc.M) string {
	if ms, ok := proj["memberships"].(pgdoc.A); ok {
		for _, m := range ms {
			mm, ok := m.(pgdoc.M)
			if !ok {
				continue
			}
			if role, _ := mm["role"].(string); role == "OWNER" {
				if sub, _ := mm["sub"].(string); sub != "" {
					return sub
				}
			}
		}
	}
	if owner, _ := proj["owner"].(string); owner != "" {
		return owner
	}
	return ""
}
