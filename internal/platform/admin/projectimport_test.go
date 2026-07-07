package admin

import (
	"net/http"
	"testing"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// projectimport_test.go — PURE unit tests for the project-import helpers (no
// datastore / network / cloud). The two handlers are thin (gate → datastore lookup → 501/insert); the
// faithful logic that is unit-testable is the error message/status, the external-service id
// resolution, and the new-project doc shape.

func TestProjectImportServiceNotFound(t *testing.T) {
	err := projectImportServiceNotFound("svc-1")
	if err.Status != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", err.Status)
	}
	if err.Code != http.StatusInternalServerError {
		t.Fatalf("code = %d, want 500", err.Code)
	}
	// Exact ServiceNotFoundException message ("Service not found: %s").
	if got, want := err.Msg, "Service not found: svc-1"; got != want {
		t.Fatalf("msg = %q, want %q", got, want)
	}
}

func TestProjectImportExternalServiceID(t *testing.T) {
	oid := pgdoc.NewID()
	tests := []struct {
		name     string
		es       pgdoc.M
		fallback string
		want     string
	}{
		{"string _id", pgdoc.M{"_id": "svc-dev"}, "fallback", "svc-dev"},
		{"generated hex _id", pgdoc.M{"_id": oid}, "fallback", oid},
		{"missing _id falls back", pgdoc.M{"name": "x"}, "fallback", "fallback"},
		{"empty string _id falls back", pgdoc.M{"_id": ""}, "fallback", "fallback"},
		{"nil doc falls back", nil, "fallback", "fallback"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := projectImportExternalServiceID(tc.es, tc.fallback); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProjectImportNewProjectDoc(t *testing.T) {
	p := &openStackImportProject{ID: "os-proj-123", Name: "Production"}
	doc := projectImportNewProjectDoc(p, "svc-dev")

	if doc["name"] != "Production" {
		t.Fatalf("name = %v, want Production", doc["name"])
	}
	if doc["status"] != "ENABLED" {
		t.Fatalf("status = %v, want ENABLED", doc["status"])
	}
	if _, has := doc["_class"]; has {
		t.Fatalf("_class = %v, want absent (rebrand dropped the discriminator)", doc["_class"])
	}

	// memberships / customInfo are non-null empties (always emitted).
	mem, ok := doc["memberships"].([]any)
	if !ok || len(mem) != 0 {
		t.Fatalf("memberships = %v, want empty []any", doc["memberships"])
	}
	ci, ok := doc["customInfo"].(pgdoc.M)
	if !ok || len(ci) != 0 {
		t.Fatalf("customInfo = %v, want empty pgdoc.M", doc["customInfo"])
	}

	// services = [{ serviceId, config:{ openstackProjectId } }].
	svcs, ok := doc["services"].([]any)
	if !ok || len(svcs) != 1 {
		t.Fatalf("services = %v, want one element", doc["services"])
	}
	svc, ok := svcs[0].(pgdoc.M)
	if !ok {
		t.Fatalf("service[0] type = %T, want pgdoc.M", svcs[0])
	}
	if svc["serviceId"] != "svc-dev" {
		t.Fatalf("serviceId = %v, want svc-dev", svc["serviceId"])
	}
	cfg, ok := svc["config"].(pgdoc.M)
	if !ok {
		t.Fatalf("config type = %T, want pgdoc.M", svc["config"])
	}
	if cfg["openstackProjectId"] != "os-proj-123" {
		t.Fatalf("openstackProjectId = %v, want os-proj-123", cfg["openstackProjectId"])
	}
}

// TestProjectImportNewProjectDoc_BlankProject confirms an empty KeystoneProject still produces a
// well-formed doc (built from possibly-empty getName()/getId()).
func TestProjectImportNewProjectDoc_BlankProject(t *testing.T) {
	doc := projectImportNewProjectDoc(&openStackImportProject{}, "svc-dev")
	if doc["name"] != "" {
		t.Fatalf("name = %v, want empty", doc["name"])
	}
	svcs := doc["services"].([]any)
	cfg := svcs[0].(pgdoc.M)["config"].(pgdoc.M)
	if cfg["openstackProjectId"] != "" {
		t.Fatalf("openstackProjectId = %v, want empty", cfg["openstackProjectId"])
	}
}
