package providers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/menlocloud/stratos/internal/cloud/client"
)

// TestProvidersSmoke is a READ-ONLY live smoke (env-gated, like client/smoke_test.go): it
// authenticates against the dev region and lists SERVER + PORT cloud resources via the
// providers. Skips unless OS_AUTH_URL is set, so `go test ./...` + CI never touch a cloud.
// Creates nothing.
func TestProvidersSmoke(t *testing.T) {
	authURL := os.Getenv("OS_AUTH_URL")
	if authURL == "" {
		t.Skip("OS_AUTH_URL not set — skipping read-only cloud providers smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	cc, err := client.New(ctx, client.Config{
		AuthURL: authURL, Region: os.Getenv("OS_REGION_NAME"),
		Username: os.Getenv("OS_USERNAME"), Password: os.Getenv("OS_PASSWORD"),
		UserDomainName: os.Getenv("OS_USER_DOMAIN_NAME"),
		ProjectName:    os.Getenv("OS_PROJECT_NAME"), ProjectDomainName: os.Getenv("OS_PROJECT_DOMAIN_NAME"),
		AppCredID: os.Getenv("OS_APPLICATION_CREDENTIAL_ID"), AppCredSecret: os.Getenv("OS_APPLICATION_CREDENTIAL_SECRET"),
	})
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	region, projectID := os.Getenv("OS_REGION_NAME"), os.Getenv("OS_PROJECT_NAME")

	servers, err := NewServerProvider(cc, region, projectID).List(ctx)
	if err != nil {
		t.Fatalf("list servers: %v", err)
	}
	for _, s := range servers {
		if s.Type != "SERVER" || s.ExternalID == "" {
			t.Fatalf("bad server cloud resource: %+v", s)
		}
	}
	t.Logf("SERVER cloud resources: %d", len(servers))

	ports, err := NewPortProvider(cc, region, projectID).List(ctx)
	if err != nil {
		t.Fatalf("list ports: %v", err)
	}
	for _, p := range ports {
		if p.Type != "PORT" || p.ExternalID == "" {
			t.Fatalf("bad port cloud resource: %+v", p)
		}
	}
	t.Logf("PORT cloud resources: %d", len(ports))
}
