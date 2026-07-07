package admin

import (
	"testing"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// TestProjectExternalID pins the services[].externalProjectId extraction the cloud-resource
// mutations scope their tenant client with (pgdoc.A/pgdoc.M typing — raw driver decode shapes).
func TestProjectExternalID(t *testing.T) {
	proj := pgdoc.M{
		"services": pgdoc.A{
			pgdoc.M{"serviceId": "es-other", "externalProjectId": "ext-other"},
			pgdoc.M{"serviceId": "es-1", "externalProjectId": "ext-1", "region": "RegionOne"},
		},
	}
	if got := projectExternalID(proj, "es-1"); got != "ext-1" {
		t.Errorf("externalProjectId=%q want ext-1", got)
	}
	if got := projectExternalID(proj, "es-none"); got != "" {
		t.Errorf("missing service should yield empty, got %q", got)
	}
	if got := projectExternalID(pgdoc.M{}, "es-1"); got != "" {
		t.Errorf("no services should yield empty, got %q", got)
	}
}
