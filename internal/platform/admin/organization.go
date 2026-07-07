package admin

// organization.go implements the MUTATIONS (and the one un-registered read, GET /{id}/members) of
// the organizations surface (/api/v1/admin/organizations) — create / update / delete + member
// add / update-role / remove. The four bare/by-id/by-billing-profile/by-member reads
// (GET /organizations, GET /organizations/{id}, GET /organizations/by-billing-profile/{bp},
// GET /organizations/by-member/{sub}) are ALREADY registered in handler.go (Routes) and are
// intentionally NOT re-registered here.
//
// Per-endpoint perms:
//   create                ADMIN_ORGANIZATION_UPDATE  (admin:organization:update)
//   update                ADMIN_ORGANIZATION_UPDATE
//   delete                ADMIN_ORGANIZATION_DELETE  (admin:organization:delete)
//   getMembers            ADMIN_ORGANIZATION_READ    (admin:organization:read)
//   updateMemberRole      ADMIN_ORGANIZATION_UPDATE
//   addMember             ADMIN_ORGANIZATION_UPDATE
//   removeMember          ADMIN_ORGANIZATION_UPDATE
//
// Response = CustomHttpResponse.single(OrganizationDto) for the org mutations / list(members) for
// getMembers. OrganizationDto extends Organization (all org fields) + projectCount/memberCount
// (primitive longs, always emitted) + a populated billingProfile (omitted when null) +
// currentUserRole/currentUserPermissions (null on the admin path → omitted).
//
// EXTERNAL INTEGRATION POINTS (NOT live): the createBillingProfile=true create branch routes through
// OrganizationService.createOrganization → BillingProfileService.createBillingProfile (billing
// orchestration not wired into admin.Handler) → 501. The addMember/removeMember project-membership
// cascade (projectManagerAdminService.add/removeUserToProject — per-project, best-effort, swallowed)
// is DEFERRED (the org-membership datastore change + the toDto response are faithful). Audit is
// deferred (// TODO(audit)).

import (
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/internal/platform/audit"
	"github.com/menlocloud/stratos/internal/platform/billing"
	"github.com/menlocloud/stratos/internal/platform/user"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// removeMemberFromOrgProjects pulls sub's membership from every project owned by the org — so a
// removed org member does not retain access to the org's projects (the org-member-removal cascade).
func (r *Repo) removeMemberFromOrgProjects(ctx context.Context, orgID, sub string) error {
	_, err := r.c(projectCollection).PullFromArray(ctx, pgdoc.M{"organizationId": orgID},
		"memberships", pgdoc.M{"sub": sub})
	return err
}

const (
	orgReadPerm   = "admin:organization:read"
	orgUpdatePerm = "admin:organization:update"
	orgDeletePerm = "admin:organization:delete"
)

// routeOrganization registers the OrganizationAdminController mutation routes (+ the un-registered
// GET /{id}/members read). The bare/by-id/by-billing-profile/by-member reads are already in
// handler.go and are NOT re-registered here.
func (h *Handler) routeOrganization(r chi.Router) {
	r.Post("/organizations", h.organizationCreate)
	r.Put("/organizations/{id}", h.organizationUpdate)
	r.Delete("/organizations/{id}", h.organizationDelete)
	r.Get("/organizations/{id}/members", h.organizationMembers)
	r.Put("/organizations/{id}/member/{userSub}/role", h.organizationUpdateMemberRole)
	r.Post("/organizations/{id}/member", h.organizationAddMember)
	r.Delete("/organizations/{id}/member/{userSub}", h.organizationRemoveMember)
}

// createOrganizationReq is the create-organization request body (name required).
type createOrganizationReq struct {
	Name                 string `json:"name"`
	Description          string `json:"description"`
	OwnerSub             string `json:"ownerSub"`
	BillingProfileID     string `json:"billingProfileId"`
	CreateBillingProfile bool   `json:"createBillingProfile"`
}

// updateOrganizationReq is the update-organization request body (all fields optional; null = no change).
type updateOrganizationReq struct {
	Name             *string `json:"name"`
	Description      *string `json:"description"`
	BillingProfileID *string `json:"billingProfileId"`
}

// addOrganizationMemberReq is the add-member request body (userId, role required).
type addOrganizationMemberReq struct {
	UserID string `json:"userId"`
	Role   string `json:"role"`
}

// updateOrganizationMemberRoleReq is the update-member-role request body (role required).
type updateOrganizationMemberRoleReq struct {
	Role string `json:"role"`
}

// organizationList returns every org as the rich
// OrganizationDto (toDto = id + memberCount + projectCount + populated billingProfile), NOT the raw
// doc. ADMIN_ORGANIZATION_READ.
func (h *Handler) organizationList(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, "admin:organization:read") {
		return
	}
	orgs, err := h.repo.ListRaw(r.Context(), "organization")
	if httpx.WriteError(w, err) {
		return
	}
	dtos := make([]pgdoc.M, 0, len(orgs))
	for _, o := range orgs {
		dto, err := h.orgToDto(r.Context(), o)
		if httpx.WriteError(w, err) {
			return
		}
		dtos = append(dtos, dto)
	}
	httpx.List(w, dtos)
}

// organizationByID returns the rich OrganizationDto, or
// 404 "Organization not found" when absent. ADMIN_ORGANIZATION_READ.
func (h *Handler) organizationByID(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, "admin:organization:read") {
		return
	}
	org, err := h.repo.OrgFindByID(r.Context(), chi.URLParam(r, "id"))
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	dto, err := h.orgToDto(r.Context(), org)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, dto)
}

// orgToDto shapes the stored org doc to its API JSON
// (_id→id, drop _class) plus projectCount, memberCount, and a populated billingProfile (when the
// org carries a billingProfileId that resolves). currentUserRole / currentUserPermissions are null
// on the admin path → omitted; customInfo is emitted as-is (the domain keeps it non-null).
func (h *Handler) orgToDto(ctx context.Context, org pgdoc.M) (pgdoc.M, error) {
	dto := shapeDoc(org)
	// shapeDoc maps _id (a string id) → id; for the projectCount/memberCount lookups we need the hex
	// string (organizationId on members/projects is the org's String id).
	hexID := orgHexID(dto["id"])
	mc, err := h.repo.OrgMemberCount(ctx, hexID)
	if err != nil {
		return nil, err
	}
	pc, err := h.repo.ProjectCountByOrganizationID(ctx, hexID)
	if err != nil {
		return nil, err
	}
	dto["memberCount"] = mc
	dto["projectCount"] = pc
	// billingProfile populated only when present (null is omitted).
	if bpID, _ := dto["billingProfileId"].(string); bpID != "" {
		bp, err := h.repo.BillingProfileByIDRaw(ctx, bpID)
		if err != nil {
			return nil, err
		}
		if bp != nil {
			dto["billingProfile"] = shapeDoc(bp)
		}
	}
	return dto, nil
}

// orgHexID renders a shaped `id` value (a string id) as the hex string used for the
// organizationId foreign key on members/projects.
func orgHexID(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// organizationCreate handles createOrganization (ADMIN_ORGANIZATION_UPDATE). Validation order is
// faithful: name not-null → (createBillingProfile && ownerSub==null) 400 → (billingProfileId &&
// createBillingProfile) 400 mutually-exclusive → resolve owner (404 when ownerSub set but missing).
// The createBillingProfile=true branch creates the owner-populated BillingProfile via
// billing.CreateForOrganization (same as client onboarding) and links it; the plain branch is
// pure datastore (save org with customInfo:{} → add OWNER member when an owner → validate a supplied
// billingProfileId).
func (h *Handler) organizationCreate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, orgUpdatePerm) {
		return
	}
	var req createOrganizationReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// Assert.notNull(request.getName(), ...) — an Assert failure is an IllegalArgumentException → 400.
	if req.Name == "" {
		httpx.WriteError(w, httpx.BadRequest("Organization name must not be null"))
		return
	}
	if req.CreateBillingProfile && req.OwnerSub == "" {
		httpx.WriteError(w, httpx.BadRequest("Owner is required when creating a billing profile"))
		return
	}
	if req.BillingProfileID != "" && req.CreateBillingProfile {
		httpx.WriteError(w, httpx.BadRequest("billing_profile_id and create_billing_profile are mutually exclusive"))
		return
	}
	// Resolve the owner (when ownerSub is supplied): getBySub → 404 "User not found with sub: <sub>".
	var owner *user.User
	if req.OwnerSub != "" {
		o, err := h.users.FindBySub(r.Context(), req.OwnerSub)
		if httpx.WriteError(w, err) {
			return
		}
		if o == nil {
			httpx.WriteError(w, httpx.NotFound("User not found with sub: "+req.OwnerSub))
			return
		}
		owner = o
	}
	// Build + save the org (customInfo defaults to {}) — both branches.
	doc := pgdoc.M{"name": req.Name, "customInfo": pgdoc.M{}}
	if req.Description != "" {
		doc["description"] = req.Description
	}
	if req.BillingProfileID != "" {
		doc["billingProfileId"] = req.BillingProfileID
	}
	saved, err := h.repo.OrgInsert(r.Context(), doc)
	if httpx.WriteError(w, err) {
		return
	}
	orgID := orgHexID(saved["_id"])
	if owner != nil {
		if err := h.repo.OrgAddMember(r.Context(), orgID, owner.Sub, "OWNER"); httpx.WriteError(w, err) {
			return
		}
	}
	switch {
	case req.CreateBillingProfile:
		// createBillingProfile=true → owner is guaranteed (validated above). Create the owner-populated
		// BillingProfile (StatusNew, base currency) and link it — mirrors the client onboarding
		// org.Service.CreateOrganization → billing.CreateForOrganization.
		bpID, err := h.billing.CreateForOrganization(r.Context(), orgID, billing.Owner{
			Sub: owner.Sub, Email: owner.Email, FirstName: owner.FirstName, LastName: owner.LastName, FullName: owner.FullName(),
		})
		if httpx.WriteError(w, err) {
			return
		}
		if _, err := h.repo.SetFields(r.Context(), "organization", orgID, pgdoc.M{"billingProfileId": bpID}); httpx.WriteError(w, err) {
			return
		}
		saved["billingProfileId"] = bpID
	case req.BillingProfileID != "":
		// A supplied billingProfileId is validated (getBillingProfileById throws if missing) — perform
		// the read to keep the side effect faithful, ignoring the populated DTO.
		if _, err := h.repo.BillingProfileByIDRaw(r.Context(), req.BillingProfileID); httpx.WriteError(w, err) {
			return
		}
	}
	// TODO(audit): auditAdmin(organization, CREATE, ORGANIZATION)
	dto, err := h.orgToDto(r.Context(), saved)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, dto)
}

// organizationUpdate handles updateOrganization (ADMIN_ORGANIZATION_UPDATE): getOrganization-or-404 →
// set name/description/billingProfileId only when the request field is non-null → save → single(dto).
func (h *Handler) organizationUpdate(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, orgUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req updateOrganizationReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	org, err := h.repo.OrgFindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	before := maps.Clone(org)
	if req.Name != nil {
		org["name"] = *req.Name
	}
	if req.Description != nil {
		org["description"] = *req.Description
	}
	if req.BillingProfileID != nil {
		org["billingProfileId"] = *req.BillingProfileID
	}
	if err := h.repo.OrgReplace(r.Context(), id, org); httpx.WriteError(w, err) {
		return
	}
	// UPDATE ORGANIZATION: field-level diff (middleware computes diffSnapshots(before, after)).
	after, _ := h.repo.OrgFindByID(r.Context(), id)
	audit.RecordSnapshots(r.Context(), before, after)
	dto, err := h.orgToDto(r.Context(), org)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, dto)
}

// organizationDelete handles deleteOrganization (ADMIN_ORGANIZATION_DELETE): getOrganization-or-404 →
// projectCount>0 400 "Cannot delete organization with associated projects..." → delete all members →
// delete the org → success("Successful operation").
func (h *Handler) organizationDelete(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, orgDeletePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	org, err := h.repo.OrgFindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	count, err := h.repo.ProjectCountByOrganizationID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if count > 0 {
		httpx.WriteError(w, httpx.BadRequest("Cannot delete organization with associated projects. Please delete or move all projects first."))
		return
	}
	if err := h.repo.OrgDeleteAllMembers(r.Context(), id); httpx.WriteError(w, err) {
		return
	}
	if err := h.repo.OrgDelete(r.Context(), id); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(organization, DELETE, ORGANIZATION)
	httpx.OK(w, "Successful operation")
}

// organizationMemberDto is the member wire shape (null names/email omitted).
type organizationMemberDto struct {
	Sub       string `json:"sub,omitempty"`
	FirstName string `json:"firstName,omitempty"`
	LastName  string `json:"lastName,omitempty"`
	Email     string `json:"email,omitempty"`
	Role      string `json:"role,omitempty"`
}

// organizationMembers handles getMembers (ADMIN_ORGANIZATION_READ): getOrganization-or-404 → for each
// membership, enrich with the User's name/email (when a User exists) → list(members).
func (h *Handler) organizationMembers(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, orgReadPerm) {
		return
	}
	id := chi.URLParam(r, "id")
	org, err := h.repo.OrgFindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	members, err := h.repo.OrgMembers(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	out := make([]organizationMemberDto, 0, len(members))
	for _, m := range members {
		sub, _ := m["sub"].(string)
		dto := organizationMemberDto{Sub: sub, Role: orgMemberRole(m)}
		if u, err := h.users.FindBySub(r.Context(), sub); err == nil && u != nil {
			dto.FirstName, dto.LastName, dto.Email = u.FirstName, u.LastName, u.Email
		}
		out = append(out, dto)
	}
	httpx.List(w, out)
}

// organizationUpdateMemberRole handles updateMemberRole (ADMIN_ORGANIZATION_UPDATE):
// getOrganization-or-404 → getMember-or-404 "User is not a member of this organization" →
// updateRole → single(dto).
func (h *Handler) organizationUpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, orgUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	userSub := chi.URLParam(r, "userSub")
	var req updateOrganizationMemberRoleReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	org, err := h.repo.OrgFindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	member, err := h.repo.OrgMember(r.Context(), id, userSub)
	if httpx.WriteError(w, err) {
		return
	}
	if member == nil {
		httpx.WriteError(w, httpx.NotFound("User is not a member of this organization"))
		return
	}
	if err := h.repo.OrgUpdateMemberRole(r.Context(), id, userSub, req.Role); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(organization, CHANGE_ROLE, ORGANIZATION, {member, newRole})
	dto, err := h.orgToDto(r.Context(), org)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, dto)
}

// organizationAddMember handles addMember (ADMIN_ORGANIZATION_UPDATE): resolve the user by _id
// (getById) → 400 "User with id <id> not found" → getOrganization-or-404 → already-member 400
// "User is already a member of this organization" → re-resolve the user by sub (getBySub) → 400
// "User with sub <sub> not found" → addMember → single(dto). The per-project membership cascade
// (projectManagerAdminService.addUserToProject, best-effort/swallowed) is DEFERRED.
func (h *Handler) organizationAddMember(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, orgUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	var req addOrganizationMemberReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	// addMemberByUserId: userService.getById(userId) → 400 "User with id <id> not found".
	memberUser, err := h.repo.UserByID(r.Context(), req.UserID)
	if httpx.WriteError(w, err) {
		return
	}
	if memberUser == nil {
		httpx.WriteError(w, httpx.BadRequest("User with id "+req.UserID+" not found"))
		return
	}
	sub := userSub(memberUser)
	// addMember: getOrganization → 404.
	org, err := h.repo.OrgFindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	// isMember → 400 "User is already a member of this organization".
	existing, err := h.repo.OrgMember(r.Context(), id, sub)
	if httpx.WriteError(w, err) {
		return
	}
	if existing != nil {
		httpx.WriteError(w, httpx.BadRequest("User is already a member of this organization"))
		return
	}
	// userService.getBySub(userSub) → 400 "User with sub <sub> not found" (re-resolve, faithful).
	resolved, err := h.users.FindBySub(r.Context(), sub)
	if httpx.WriteError(w, err) {
		return
	}
	if resolved == nil {
		httpx.WriteError(w, httpx.BadRequest("User with sub "+sub+" not found"))
		return
	}
	if err := h.repo.OrgAddMember(r.Context(), id, sub, req.Role); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): auditAdmin(organization, ADD_MEMBER, ORGANIZATION, {member, role})
	// TODO(cascade): projectManagerAdminService.addUserToProject for each org project (best-effort).
	dto, err := h.orgToDto(r.Context(), org)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, dto)
}

// organizationRemoveMember handles removeMember (ADMIN_ORGANIZATION_UPDATE): getOrganization-or-404 →
// isUserOwner 400 "Cannot remove organization owner" → removeMember → single(dto). The per-project
// membership-removal cascade (projectManagerAdminService.removeUserFromProject, best-effort/
// swallowed) is DEFERRED.
func (h *Handler) organizationRemoveMember(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, orgUpdatePerm) {
		return
	}
	id := chi.URLParam(r, "id")
	userSub := chi.URLParam(r, "userSub")
	org, err := h.repo.OrgFindByID(r.Context(), id)
	if httpx.WriteError(w, err) {
		return
	}
	if org == nil {
		httpx.WriteError(w, httpx.NotFound("Organization not found"))
		return
	}
	// isUserOwner: the membership's role == OWNER → 400 "Cannot remove organization owner".
	member, err := h.repo.OrgMember(r.Context(), id, userSub)
	if httpx.WriteError(w, err) {
		return
	}
	if member != nil && orgMemberRole(member) == "OWNER" {
		httpx.WriteError(w, httpx.BadRequest("Cannot remove organization owner"))
		return
	}
	if err := h.repo.OrgRemoveMember(r.Context(), id, userSub); httpx.WriteError(w, err) {
		return
	}
	// Cascade: a removed org member must also lose their memberships on the org's projects — else
	// they retain project access after being removed from the org. Best-effort (mirrors
	// projectManagerAdminService.removeUserFromProject, swallowed).
	if err := h.repo.removeMemberFromOrgProjects(r.Context(), id, userSub); err != nil {
		slog.Error("cascade remove org member from org projects failed", "org", id, "sub", userSub, "err", err)
	}
	// TODO(audit): auditAdmin(organization, REMOVE_MEMBER, ORGANIZATION, {member})
	dto, err := h.orgToDto(r.Context(), org)
	if httpx.WriteError(w, err) {
		return
	}
	httpx.OK(w, dto)
}
