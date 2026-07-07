package client

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestCloudSmoke is a READ-ONLY connectivity smoke against a real OpenStack region.
// It authenticates and lists flavors/images/networks — no resource is created. Skipped
// unless OS_AUTH_URL is set, so the normal `go test ./...` (and CI) never touch a cloud.
//
// Run locally against the dev region (creds in memory openstack-dev-region):
//
//	OS_AUTH_URL=https://cloud-console.menlo.ai:5000/v3 OS_REGION_NAME=RegionOne \
//	OS_USERNAME=dev OS_PASSWORD=*** OS_USER_DOMAIN_NAME=Default \
//	OS_PROJECT_NAME=dev OS_PROJECT_DOMAIN_NAME=Default \
//	go test ./internal/cloud/client -run TestCloudSmoke -v
func TestCloudSmoke(t *testing.T) {
	if os.Getenv("OS_AUTH_URL") == "" {
		t.Skip("OS_AUTH_URL not set — skipping read-only cloud smoke")
	}
	cfg := Config{
		AuthURL:           os.Getenv("OS_AUTH_URL"),
		Region:            os.Getenv("OS_REGION_NAME"),
		Username:          os.Getenv("OS_USERNAME"),
		Password:          os.Getenv("OS_PASSWORD"),
		UserDomainName:    os.Getenv("OS_USER_DOMAIN_NAME"),
		ProjectName:       os.Getenv("OS_PROJECT_NAME"),
		ProjectDomainName: os.Getenv("OS_PROJECT_DOMAIN_NAME"),
		AppCredID:         os.Getenv("OS_APPLICATION_CREDENTIAL_ID"),
		AppCredSecret:     os.Getenv("OS_APPLICATION_CREDENTIAL_SECRET"),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	c, err := New(ctx, cfg)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}

	flavors, err := c.ListFlavors(ctx)
	if err != nil {
		t.Fatalf("list flavors: %v", err)
	}
	t.Logf("flavors: %d", len(flavors))
	if len(flavors) == 0 {
		t.Error("expected at least one flavor")
	}

	imgs, err := c.ListImages(ctx)
	if err != nil {
		t.Fatalf("list images: %v", err)
	}
	t.Logf("images: %d", len(imgs))

	nets, err := c.ListNetworks(ctx)
	if err != nil {
		t.Fatalf("list networks: %v", err)
	}
	t.Logf("networks: %d", len(nets))
	if len(nets) == 0 {
		t.Error("expected at least one network")
	}
}
