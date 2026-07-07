package admin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"golang.org/x/crypto/bcrypt"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/pkg/httpx"
)

// usermanagement.go implements the user-management surface (/api/v1/admin/user-management).
// Follows the custommenu.go/banktransfer.go references: exact
// perms / error strings / response envelopes, id-aware CRUD via the crud.go helpers,
// `_id`→`id` shaping on the way out.
//
// Call graph:
//
//	GET    /credentials?sub=          listCredentialsBySub(sub) = userCredentialRepository.findAllBySub(sub)
//	                                   → UserCredentialAdminDto.toDto (ADMIN_USER_READ)
//	DELETE /credentials/{id}?sub=     removeCredential(sub, id): findById-or-404 → sub-mismatch 400
//	                                   → delete (ADMIN_USER_MANAGE_CREDENTIALS)
//	PUT    /password?sub=             updateUserPassword(sub, newPassword): findBySub-or-404 →
//	                                   userCredentialService.replacePassword (ADMIN_USER_MANAGE_CREDENTIALS)
//
// All three legs are pure datastore over the userCredential table and are handled FULLY —
// updateUserPassword's replacePassword (UserCredentialService: delete all PASSWORD creds for the
// sub + save a fresh bcrypt-hashed one) is the LOCAL password store, not an external identity
// call (the local auth = an embedded authorization server over these same docs). The
// deployed login path is Keycloak, so this store is reference-state only.
//
// The service also writes audit events (auditService.logAsync / auditAdmin) on remove + password-reset —
// deferred this pass (// TODO(audit)); the persisted state + response are faithful.

const userManagementReadPerm = "admin:user:read"
const userManagementManageCredentialsPerm = "admin:user:manage_credentials"

// userCredentialCollection is the default collection for the UserCredential domain (no
// document-name override → uncapitalized class name).
const userCredentialCollection = "userCredential"

// replacePasswordCredential replaces the password credential: delete every PASSWORD
// credential for the sub, then save {sub, type:PASSWORD, password:{hash}, createdAt}.
func (r *Repo) replacePasswordCredential(ctx context.Context, sub, hash string) error {
	if _, err := r.c(userCredentialCollection).DeleteMany(ctx,
		pgdoc.M{"sub": sub, "type": "PASSWORD"}); err != nil {
		return err
	}
	_, err := r.c(userCredentialCollection).InsertOne(ctx, pgdoc.M{
		"sub":       sub,
		"type":      "PASSWORD",
		"password":  pgdoc.M{"hash": hash},
		"createdAt": time.Now().UTC(),
	})
	return err
}

// routeUserManagement registers the user-management routes. None of these are
// registered in handler.go (the /user-management prefix is new).
func (h *Handler) routeUserManagement(r chi.Router) {
	r.Get("/user-management/credentials", h.userManagementListCredentials)
	r.Delete("/user-management/credentials/{credentialId}", h.userManagementRemoveCredential)
	r.Put("/user-management/password", h.userManagementUpdatePassword)
}

// userCredentialPasswordDto is the password sub-object: configured = the credential
// has a non-null password.hash. A primitive bool → always emitted.
type userCredentialPasswordDto struct {
	Configured bool `json:"configured"`
}

// userCredentialTotpDto is the totp sub-object: verified (primitive, always emitted)
// + deviceName (nullable → omitted when blank).
type userCredentialTotpDto struct {
	Verified   bool   `json:"verified"`
	DeviceName string `json:"deviceName,omitempty"`
}

// userCredentialAdminDto is the credential wire shape. password/totp are pointers so a null
// sub-object is omitted, and toDto only builds them when the source
// field is non-null. createdAt/updatedAt are passed through as the stored value (the
// normalizer masks `*At`); omitted when absent.
type userCredentialAdminDto struct {
	ID        any                        `json:"id,omitempty"`
	Sub       string                     `json:"sub,omitempty"`
	Type      string                     `json:"type,omitempty"`
	Password  *userCredentialPasswordDto `json:"password,omitempty"`
	Totp      *userCredentialTotpDto     `json:"totp,omitempty"`
	CreatedAt any                        `json:"createdAt,omitempty"`
	UpdatedAt any                        `json:"updatedAt,omitempty"`
}

// userCredentialToDto maps a raw userCredential doc to its wire shape:
//   - password sub-object present (non-null) → {configured: password.hash != null}
//   - totp sub-object present (non-null)     → {verified, deviceName}
//
// `id` is the stored `_id` (a plain hex string). The `_class`
// discriminator and the raw password/totp/webAuthn material never reach the wire.
func userCredentialToDto(doc pgdoc.M) userCredentialAdminDto {
	dto := userCredentialAdminDto{
		Sub:       asString(doc["sub"]),
		Type:      asString(doc["type"]),
		CreatedAt: doc["createdAt"],
		UpdatedAt: doc["updatedAt"],
	}
	if v, ok := doc["_id"]; ok {
		// Pass the raw _id through: it is a plain hex string id that marshals as-is.
		dto.ID = v
	}
	if pw, ok := doc["password"].(pgdoc.M); ok && pw != nil {
		_, hasHash := pw["hash"]
		dto.Password = &userCredentialPasswordDto{Configured: hasHash && pw["hash"] != nil}
	}
	if totp, ok := doc["totp"].(pgdoc.M); ok && totp != nil {
		dto.Totp = &userCredentialTotpDto{
			Verified:   asBool(totp["verified"]),
			DeviceName: asString(totp["deviceName"]),
		}
	}
	return dto
}

// asString reads a stored value as a string ("" when absent / non-string; a fmt.Stringer is
// rendered via its String()). Kept local to avoid touching the shared crud.go helpers.
func asString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case fmt.Stringer:
		return s.String()
	default:
		return ""
	}
}

// asBool reads a stored value as a bool (false when absent / non-bool).
func asBool(v any) bool {
	b, _ := v.(bool)
	return b
}

// userManagementListCredentials handles listCredentials: findAllBySub(sub) → DTO list (ADMIN_USER_READ).
func (h *Handler) userManagementListCredentials(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userManagementReadPerm) {
		return
	}
	sub := r.URL.Query().Get("sub")
	creds, err := h.repo.ListRawFiltered(r.Context(), userCredentialCollection, pgdoc.M{"sub": sub})
	if httpx.WriteError(w, err) {
		return
	}
	dtos := make([]userCredentialAdminDto, 0, len(creds))
	for _, c := range creds {
		dtos = append(dtos, userCredentialToDto(c))
	}
	httpx.List(w, dtos)
}

// userManagementRemoveCredential handles removeCredential (ADMIN_USER_MANAGE_CREDENTIALS):
// findById-or-404 "Credential not found" → sub-mismatch 400 "Credential does not belong to the
// specified user" → delete → success("Successful operation").
func (h *Handler) userManagementRemoveCredential(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userManagementManageCredentialsPerm) {
		return
	}
	sub := r.URL.Query().Get("sub")
	credentialID := chi.URLParam(r, "credentialId")
	existing, err := h.repo.FindDoc(r.Context(), userCredentialCollection, credentialID)
	if httpx.WriteError(w, err) {
		return
	}
	if existing == nil {
		httpx.WriteError(w, httpx.NotFound("Credential not found"))
		return
	}
	if asString(existing["sub"]) != sub {
		httpx.WriteError(w, httpx.BadRequest("Credential does not belong to the specified user"))
		return
	}
	if _, err := h.repo.DeleteDoc(r.Context(), userCredentialCollection, credentialID); httpx.WriteError(w, err) {
		return
	}
	// TODO(audit): logAsync(adminEventFromContext DELETE USER resourceId=sub
	//              metadata{credentialId,credentialType} SUCCESS)
	httpx.OK(w, "Successful operation")
}

// userManagementUpdatePassword handles updateUserPassword (ADMIN_USER_MANAGE_CREDENTIALS):
// findBySubOrderByCreatedAtDesc-or-404 "User not found", then replacePassword (bcrypt-hash
// + rewrite the PASSWORD credential, an identity op). The body's newPassword is required
// (validation 400 on blank) — replicated as a 400 before the write.
func (h *Handler) userManagementUpdatePassword(w http.ResponseWriter, r *http.Request) {
	if !h.require(w, r, userManagementManageCredentialsPerm) {
		return
	}
	var req struct {
		NewPassword string `json:"newPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httpx.WriteError(w, httpx.BadRequest("Invalid request body"))
		return
	}
	if req.NewPassword == "" {
		// newPassword is required (message = "New password is required").
		httpx.WriteError(w, httpx.BadRequest("New password is required"))
		return
	}
	sub := r.URL.Query().Get("sub")
	u, err := h.users.FindBySub(r.Context(), sub)
	if httpx.WriteError(w, err) {
		return
	}
	if u == nil {
		httpx.WriteError(w, httpx.NotFound("User not found"))
		return
	}
	// UserCredentialService.replacePassword: delete every PASSWORD credential for the sub, then
	// save a fresh one {sub, type:PASSWORD, password:{hash:<bcrypt>}, createdAt} (the
	// password encoder is BCrypt). Persisted-state faithful; note the DEPLOYED login
	// path is Keycloak, so this credential doc is the reference store, not the live login secret.
	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if httpx.WriteError(w, err) {
		return
	}
	if err := h.repo.replacePasswordCredential(r.Context(), sub, string(hash)); httpx.WriteError(w, err) {
		return
	}
	// PASSWORD_RESET audit (auditAdmin PLATFORM — the middleware emits the admin event).
	httpx.OK(w, "Successful operation")
}
