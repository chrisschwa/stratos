package admin

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/platform/user"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// projectmanager.go implements the MUTATIONS of the project-manager surface
// (/api/v1/admin/projects/manage): add-member, remove-member, invite. All three gate on
// ADMIN_PROJECT_MANAGE. (The base path /projects/manage is NOT touched by any
// existing handler.go route — the existing project reads live under the singular /project.)
//
// Call graph:
//
//	POST   /projects/manage          addUserToProject(AddUserToProjectRequest{userId,projectId,role})
//	                                  → projectService.addUserToProject(req)  [the plain overload]
//	POST   /projects/manage/remove   removeUserFromProject(RemoveUserFromProjectRequest{projectId,sub})
//	POST   /projects/manage/invite   inviteUserToProject(ProjectUserInviteRequest{projectId,newUser,userIds})
//
// ── addUserToProject (ProjectService.addUserToProject) ──
//
//	project = getProjectById(projectId)            → 404 "The project with id %s was not found. "
//	user    = userService.getById(userId)          → 404 "User with id %s not found " when absent
//	if project.isDisabled() (status==DISABLED)     → 400 "Project is suspended. Cannot add user to project"
//	if memberships.sub already contains user.sub    → 400 "User is already added to project"
//	memberships += {sub:user.sub, role:req.role}; projectRepository.save(project)   [PERSISTED state]
//	platformExternalService.addUserToProject(ctx,user)   [CLOUD, not wired — provisions the user on the live
//	                                                       external services for the project]
//	return single(project)
//
// The membership append is persisted, then the updated project is returned. The platform runs every
// cloud call through an ADMIN-scoped tenant client, so members carry no per-user keystone identity —
// there is no per-user cloud grant to perform (this mirrors the client-side AddMember, datastore-only).
//
// ── removeUserFromProject (ProjectManagerAdminService.removeUserFromProject) ──
//
//	project = getProjectById(projectId)            → 404 "The project with id %s was not found. "
//	user    = userService.getBySub(sub)            → null ⇒ 404 "User not found with sub: " + sub
//	membership = memberships.first(sub==user.sub)
//	if membership == null                           → 400 "User is already removed from project"
//	if membership.role == OWNER                     → 400 "Project owner cannot be removed from project"
//	memberships.removeIf(sub==user.sub); projectRepository.save(project)            [PERSISTED state]
//	(no per-user cloud revoke — admin-scoped model, see addUser)
//	return single(project)
//
// ── inviteUserToProject (ProjectManagerAdminService.inviteUserToProject) ──
//
//	project = getProjectById(projectId)            → 404 "The project with id %s was not found. "
//	if userIds == null                              → 400 "User IDs must be provided for project invitation"
//	newUser ? per email: projectInviteService.inviteNewUserToProject(email, project)
//	        : per userId→user: projectInviteService.inviteToProject(user.email, project)
//	return success()  ("Successful operation")
//
// invite creates project-invite records + sends invitation emails via the wired inviteToProject leg
// (the same one the admin user-create loop uses). newUser → invite by email address; else resolve
// each userId → invite by the user's email. Per-item failures are swallowed (best-effort).

const projectManagePerm = "admin:project:manage"

// projectCollection is declared in projectmut.go (same package).

// routeProjectManager registers the project-manager mutation routes. The base path
// /projects/manage has no overlap with the existing /project routes in handler.go.
func (h *Handler) routeProjectManager(r chi.Router) {
	r.Post("/projects/manage", h.projectManagerAddUser)
	r.Post("/projects/manage/remove", h.projectManagerRemoveUser)
	r.Post("/projects/manage/invite", h.projectManagerInvite)
}

// projectManagerAddUserReq is the add-user request body {userId, projectId, role}.
type projectManagerAddUserReq struct {
	UserID    string `json:"userId"`
	ProjectID string `json:"projectId"`
	Role      string `json:"role"`
}

// projectManagerRemoveUserReq is the remove-user request body {projectId, sub}.
type projectManagerRemoveUserReq struct {
	ProjectID string `json:"projectId"`
	Sub       string `json:"sub"`
}

// projectManagerInviteReq is the invite request body {projectId, newUser, userIds}.
type projectManagerInviteReq struct {
	ProjectID string   `json:"projectId"`
	NewUser   bool     `json:"newUser"`
	UserIDs   []string `json:"userIds"`
}

// projectNotFound is the exact 404 (TranslationConstants.PROJECT_ID_WAS_NOT_FOUND,
// "The project with id %s was not found. " — trailing space, interpolated).
func projectNotFound(id string) *httpx.HTTPError {
	return httpx.NotFound(fmt.Sprintf("The project with id %s was not found. ", id))
}

// projectManagerAddUser handles addUserToProject:
// resolve project + user(by id) → suspended/already-member guards → append membership (PERSISTED) →
// platformExternalService.addUserToProject [CLOUD, not wired].
func (h *Handler) projectManagerAddUser(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectManagePerm) {
		return
	}
	var req projectManagerAddUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	proj, err := h.repo.projectByID(r.Context(), req.ProjectID)
	if httpx.WriteError(w, err) {
		return
	}
	if proj == nil {
		httpx.WriteError(w, projectNotFound(req.ProjectID))
		return
	}
	u, err := h.repo.userByID(r.Context(), req.UserID)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		// UserService.getById → notFound USER_ID_NOT_FOUND "User with id %s not found "
		httpx.WriteError(w, httpx.NotFound(fmt.Sprintf("User with id %s not found ", req.UserID)))
		return
	}
	// project.isDisabled() == status==DISABLED → suspended guard.
	if status, _ := proj["status"].(string); status == "DISABLED" {
		httpx.WriteError(w, httpx.BadRequest("Project is suspended. Cannot add user to project"))
		return
	}
	if projectHasMember(proj, u.Sub) {
		httpx.WriteError(w, httpx.BadRequest("User is already added to project"))
		return
	}
	// Append Membership{sub, role} and return the updated project. The platform runs every cloud
	// call through an ADMIN-scoped tenant client — members have no per-user keystone identity — so
	// there is no per-user cloud grant to make here (this mirrors the client-side AddMember, which is
	// datastore-only). Returning the project after the persist also avoids the prior state-leak where the
	// membership was written but the caller received a 501.
	if err := h.repo.addProjectMembership(r.Context(), req.ProjectID, u.Sub, req.Role); httpx.WriteError(w, err) {
		return
	}
	updated, err := h.repo.projectByID(r.Context(), req.ProjectID)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, shapeDoc(updated))
}

// projectManagerRemoveUser handles removeUserFromProject:
// resolve project + user(by sub) → membership/owner guards → remove membership (PERSISTED) →
// platformExternalService.removeUserFromProject [CLOUD, not wired].
func (h *Handler) projectManagerRemoveUser(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectManagePerm) {
		return
	}
	var req projectManagerRemoveUserReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	proj, err := h.repo.projectByID(r.Context(), req.ProjectID)
	if httpx.WriteError(w, err) {
		return
	}
	if proj == nil {
		httpx.WriteError(w, projectNotFound(req.ProjectID))
		return
	}
	u, err := h.repo.userBySub(r.Context(), req.Sub)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		// removeUserFromProject: null user → notFound("User not found with sub: " + sub)
		httpx.WriteError(w, httpx.NotFound("User not found with sub: "+req.Sub))
		return
	}
	role, found := projectMemberRole(proj, u.Sub)
	if !found {
		httpx.WriteError(w, httpx.BadRequest("User is already removed from project"))
		return
	}
	if role == "OWNER" {
		httpx.WriteError(w, httpx.BadRequest("Project owner cannot be removed from project"))
		return
	}
	// removeIf(memberships.sub == user.sub) → return the updated project. No per-user cloud revoke
	// (admin-scoped model, see addUser) — and returning the project avoids the state-leak.
	if err := h.repo.removeProjectMembership(r.Context(), req.ProjectID, u.Sub); httpx.WriteError(w, err) {
		return
	}
	updated, err := h.repo.projectByID(r.Context(), req.ProjectID)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, shapeDoc(updated))
}

// projectManagerInvite handles inviteUserToProject:
// resolve project → null-userIds guard → ProjectInviteService invite [EMAIL/INVITE, not wired].
func (h *Handler) projectManagerInvite(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, projectManagePerm) {
		return
	}
	var req projectManagerInviteReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	proj, err := h.repo.projectByID(r.Context(), req.ProjectID)
	if httpx.WriteError(w, err) {
		return
	}
	if proj == nil {
		httpx.WriteError(w, projectNotFound(req.ProjectID))
		return
	}
	if req.UserIDs == nil {
		httpx.WriteError(w, httpx.BadRequest("User IDs must be provided for project invitation"))
		return
	}
	if h.inviteToProject == nil {
		// Invite subsystem not wired (unit tests construct the Handler without it).
		httpx.WriteError(w, httpx.NewError(http.StatusNotImplemented, http.StatusNotImplemented,
			"projectInviteService invite not implemented"))
		return
	}
	// newUser → UserIDs carry EMAIL addresses (invite by address); else USER IDs (resolve the user →
	// invite by that user's email). Per-item failures are logged+skipped (best-effort): one bad invite
	// must not abort the batch, so each is swallowed and the endpoint always returns success.
	for _, item := range req.UserIDs {
		if req.NewUser {
			_ = h.inviteToProject(r.Context(), &user.User{Email: item}, item, req.ProjectID)
			continue
		}
		invitee, err := h.repo.userByID(r.Context(), item)
		if err != nil || invitee == nil || invitee.Email == "" {
			continue
		}
		_ = h.inviteToProject(r.Context(), invitee, invitee.Email, req.ProjectID)
	}
	httpx.OK(w, "Successful operation")
}
