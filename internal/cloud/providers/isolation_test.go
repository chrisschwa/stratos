package providers

import (
	"testing"

	"github.com/menlocloud/stratos/internal/cloud"
)

// §6: resolveExtID must only resolve a secondary body id against a cache row OWNED by the acting
// project. crOwnedBy is that gate — a raw external id (no cache row) or a row owned by another
// project must be rejected so a create/action can't reach across to another tenant's resource by id.
func TestCrOwnedBy_rejectsForeignAndRawIDs(t *testing.T) {
	const proj = "proj-A"

	owned := &cloud.CloudResource{ExternalID: "net-ext-1", ProjectID: proj}
	if !crOwnedBy(owned, proj) {
		t.Fatal("an owned cache row must resolve")
	}

	foreign := &cloud.CloudResource{ExternalID: "net-ext-2", ProjectID: "proj-B"}
	if crOwnedBy(foreign, proj) {
		t.Fatal("§6: a cache row owned by another project must be rejected")
	}

	if crOwnedBy(nil, proj) {
		t.Fatal("§6: a raw external id (no cache row) must be rejected")
	}

	noExt := &cloud.CloudResource{ProjectID: proj}
	if crOwnedBy(noExt, proj) {
		t.Fatal("a row with no externalId must be rejected")
	}
}
