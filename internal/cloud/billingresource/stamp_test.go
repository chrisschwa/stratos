package billingresource

import (
	"testing"
	"time"

	"github.com/menlocloud/stratos/internal/cloud"
	"github.com/menlocloud/stratos/internal/platform/pricing"
)

func TestStampResourceValues(t *testing.T) {
	info := time.Date(2026, 6, 1, 8, 0, 0, 0, time.UTC)
	doc := time.Date(2026, 5, 20, 0, 0, 0, 0, time.UTC)

	// Info.CreatedAt preferred over doc createdAt; region/service_id stamped + attrs declared.
	withType := &pricing.BillingResource{BillingResourceType: &pricing.BillingResourceType{ResourceType: "instance"}}
	noType := &pricing.BillingResource{}
	cr := &cloud.CloudResource{Region: "RegionOne", ServiceID: "svc-x", CreatedAt: &doc, Info: &cloud.Info{CreatedAt: &info}}

	stampResourceValues([]*pricing.BillingResource{withType, noType}, cr)

	if withType.Values["region"] != "RegionOne" || withType.Values["service_id"] != "svc-x" {
		t.Errorf("region/service_id not stamped: %#v", withType.Values)
	}
	if withType.CreatedAt == nil || !withType.CreatedAt.Equal(info) {
		t.Errorf("CreatedAt = %v, want Info.CreatedAt %v", withType.CreatedAt, info)
	}
	if got, _ := withType.BillingResourceType.AttributeTypeByName("region"); got != "string" {
		t.Errorf("region attr not declared as string")
	}
	if got, _ := withType.BillingResourceType.AttributeTypeByName("service_id"); got != "string" {
		t.Errorf("service_id attr not declared as string")
	}
	// nil-type resource: createdAt still stamped, region/service_id left unset (no type to declare on).
	if noType.CreatedAt == nil || noType.Values["region"] != nil {
		t.Errorf("nil-type should stamp createdAt but not region: %#v", noType.Values)
	}

	// Idempotent: re-stamping does not duplicate the attrs.
	stampResourceValues([]*pricing.BillingResource{withType}, cr)
	n := 0
	for _, a := range withType.BillingResourceType.Attributes {
		if a.Name == "region" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("region attr duplicated: count=%d", n)
	}

	// Fallback to doc createdAt when Info is absent.
	noInfo := &pricing.BillingResource{BillingResourceType: &pricing.BillingResourceType{}}
	stampResourceValues([]*pricing.BillingResource{noInfo}, &cloud.CloudResource{CreatedAt: &doc})
	if noInfo.CreatedAt == nil || !noInfo.CreatedAt.Equal(doc) {
		t.Errorf("fallback CreatedAt = %v, want doc %v", noInfo.CreatedAt, doc)
	}
}
