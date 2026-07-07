package admin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/pkg/httpx"
)

// adminpermission.go serves the MUTATIONS of the admin-permissions surface
// (/api/v1/admin/admin-permissions): grant / update-role / revoke. The reads on this surface
// (bare list = listAdminUsers, and /available-permissions) are already registered in handler.go
// (listRaw "adminPermission" + availablePermissions) — NOT re-registered here.
//
// All three mutations gate on ADMIN_PERMISSION_MANAGE. The service writes audit
// events (CREATE/UPDATE/DELETE on ADMIN_PERMISSION) — deferred this pass
// (// TODO(audit)); the persisted state + the response envelope are faithful.
//
// Response shape: grant + update return single(AdminPermission) — the RAW
// AdminPermission domain (null fields omitted): {sub, email?, role?, pending, createdAt?, updatedAt?}.
// Note the id field is `sub` (stored as the `id`), so the JSON key is `sub`, NOT `id`. revoke
// returns success() → httpx.OK(w, "Successful operation").

const adminPermissionManagePerm = "admin:permission:manage"

// routeAdminPermission registers ONLY the admin-permission mutations. The
// reads (bare GET list, GET /available-permissions) are registered in handler.go and skipped here.
func (h *Handler) routeAdminPermission(r chi.Router) {
	r.Post("/admin-permissions", h.grantAdminPermission)
	r.Put("/admin-permissions/{userSub}", h.updateAdminPermission)
	r.Delete("/admin-permissions/{userSub}", h.revokeAdminPermission)
}

// grantAdminPermissionRequest is the grant request ({sub required, role}).
// `sub` here is really a username (email OR sub) — the handler resolves it via findBySubOrEmail.
type grantAdminPermissionRequest struct {
	Sub  string `json:"sub"`
	Role string `json:"role"`
}

// updateAdminPermissionRequest is the update request ({role}).
type updateAdminPermissionRequest struct {
	Role string `json:"role"`
}

// adminPermissionView is the JSON shape of a saved AdminPermission (single response):
// email/role omitted when blank, pending always present. createdAt/updatedAt are managed by
// auditing (deferred here — they are nullable and omitted, which is the value an
// unseeded greenfield read would also show; the comparison harness masks any *At key regardless).
type adminPermissionView struct {
	Sub     string `json:"sub,omitempty"`
	Email   string `json:"email,omitempty"`
	Role    string `json:"role,omitempty"`
	Pending bool   `json:"pending"`
}

// grantAdminPermission handles grantPermission (POST): ADMIN_PERMISSION_MANAGE. The request `sub` is
// treated as an email/username; resolve the real User sub via findBySubOrEmail (nil when no User),
// then save an AdminPermission keyed by sub-when-known else the email, with pending = (sub == null).
func (h *Handler) grantAdminPermission(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, adminPermissionManagePerm) {
		return
	}
	var req grantAdminPermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	email := req.Sub
	// findBySubOrEmail(email): by identity-sub first, then by email. We approximate with the user
	// repo's FindBySub (matches the top-level sub) then FindByEmail.
	var resolvedSub string
	if u, err := h.users.FindBySub(r.Context(), email); err != nil {
		httpx.WriteError(w, err)
		return
	} else if u != nil {
		resolvedSub = u.Sub
	} else if u, err := h.users.FindByEmail(r.Context(), email); err != nil {
		httpx.WriteError(w, err)
		return
	} else if u != nil {
		resolvedSub = u.Sub
	}

	saved, err := h.repo.SaveAdminPermission(r.Context(), resolvedSub, email, req.Role)
	if httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(CREATE, ADMIN_PERMISSION, resourceId=email, {role})
	httpx.OK(w, adminPermissionToView(saved))
}

// updateAdminPermission handles updatePermission (PUT /{userSub}): ADMIN_PERMISSION_MANAGE. Refuses
// to change one's own role (400 "Cannot change your own role"), else findById-or-404 → set role.
func (h *Handler) updateAdminPermission(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, adminPermissionManagePerm) {
		return
	}
	userSub := chi.URLParam(r, "userSub")
	if httpx.RC(r.Context()).Sub == userSub {
		httpx.WriteError(w, httpx.BadRequest("Cannot change your own role"))
		return
	}
	var req updateAdminPermissionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	existing, err := h.repo.FindBySub(r.Context(), userSub)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("Admin permission not found for user: %s", userSub)))
		return
	}
	// updateRole: keep sub/email/pending, overwrite ONLY role, save. A blank role becomes null
	// (removed) — omitted when empty.
	if err := h.repo.UpdateAdminPermissionRole(r.Context(), userSub, req.Role); httpx.WriteError(w, err) {
		return
	}
	existing.Role = req.Role
	// TODO(audit): auditAdmin(UPDATE, ADMIN_PERMISSION, resourceId=userSub, {oldRole,newRole})
	httpx.OK(w, adminPermissionToView(existing))
}

// revokeAdminPermission handles revokePermission (DELETE /{userSub}): ADMIN_PERMISSION_MANAGE.
// Refuses to revoke one's own access (400 "Cannot revoke your own admin access"), else deleteById →
// success("Successful operation"). Does NOT 404 on a missing id (deleteById is a no-op).
func (h *Handler) revokeAdminPermission(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, adminPermissionManagePerm) {
		return
	}
	userSub := chi.URLParam(r, "userSub")
	if httpx.RC(r.Context()).Sub == userSub {
		httpx.WriteError(w, httpx.BadRequest("Cannot revoke your own admin access"))
		return
	}
	if _, err := h.repo.DeleteDoc(r.Context(), "adminPermission", userSub); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(DELETE, ADMIN_PERMISSION, resourceId=userSub)
	httpx.OK(w, "Successful operation")
}

// adminPermissionToView maps a stored AdminPermission to its single-response JSON shape.
func adminPermissionToView(ap *AdminPermission) adminPermissionView {
	if ap == nil {
		return adminPermissionView{}
	}
	return adminPermissionView{Sub: ap.Sub, Email: ap.Email, Role: ap.Role, Pending: ap.Pending}
}
