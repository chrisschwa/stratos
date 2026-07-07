package metrics

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/menlocloud/stratos/internal/cloud/client"
)

func decFromStr(t *testing.T, s string) decimal.Decimal {
	t.Helper()
	d, err := decimal.NewFromString(s)
	if err != nil {
		t.Fatalf("bad decimal %q: %v", s, err)
	}
	return d
}

// TestMeasuresDeltaMB checks the (max−min)/1048576 decimal64 usage math, with no cloud.
func TestMeasuresDeltaMB(t *testing.T) {
	cases := []struct {
		name string
		rows [][]any
		want string
	}{
		{"empty", nil, "0"},
		{"single", [][]any{{"t", 300.0, 4000000.0}}, "0"}, // max==min → 0
		{"delta", [][]any{
			{"t1", 300.0, 1000000.0},
			{"t2", 300.0, 5000000.0},
			{"t3", 300.0, 3000000.0},
		}, "3.814697265625"}, // (5e6-1e6)/1048576
		{"string-values", [][]any{
			{"t1", 300.0, "10485760"},
			{"t2", 300.0, "20971520"},
		}, "10"}, // (20971520-10485760)/1048576 = 10
		{"skip-nil-and-short", [][]any{
			{"t1", 300.0, nil},
			{"t2"},
			{"t3", 300.0, 2097152.0},
			{"t4", 300.0, 1048576.0},
		}, "1"}, // (2097152-1048576)/1048576 = 1
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := measuresDeltaMB(c.rows)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if !got.Equal(decFromStr(t, c.want)) {
				t.Fatalf("got %s, want %s", got.String(), c.want)
			}
		})
	}
}

// TestGnocchiSmoke is a READ-ONLY live check (GET /v1/metric?limit=1) — skipped unless
// OS_AUTH_URL is set; creates nothing.
func TestGnocchiSmoke(t *testing.T) {
	if os.Getenv("OS_AUTH_URL") == "" {
		t.Skip("OS_AUTH_URL not set — skipping read-only gnocchi smoke")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cc, err := client.New(ctx, client.Config{
		AuthURL: os.Getenv("OS_AUTH_URL"), Region: os.Getenv("OS_REGION_NAME"),
		Username: os.Getenv("OS_USERNAME"), Password: os.Getenv("OS_PASSWORD"),
		UserDomainName: os.Getenv("OS_USER_DOMAIN_NAME"),
		ProjectName:    os.Getenv("OS_PROJECT_NAME"), ProjectDomainName: os.Getenv("OS_PROJECT_DOMAIN_NAME"),
	})
	if err != nil {
		t.Fatalf("auth: %v", err)
	}
	g, err := New(cc)
	if err != nil {
		t.Fatalf("gnocchi endpoint: %v", err)
	}
	t.Logf("gnocchi base: %s", g.base)
	if err := g.Ping(ctx); err != nil {
		t.Fatalf("gnocchi ping: %v", err)
	}
}
