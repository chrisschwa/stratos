package project

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Routes must register without chi pattern panics (params share tree nodes) and the 2026-07-02
// gap-scan additions must match their expected paths.
func TestRoutes_registerAndMatch(t *testing.T) {
	r := chi.NewRouter()
	(&Handler{}).Routes(r)
	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/openstack/p1/image/i1/upload"}, // canonical openstack path
		{http.MethodPost, "/project/p1/image/i1/upload"},   // pre-scan alias kept
		{http.MethodGet, "/project/p1/service/svc1"},
		{http.MethodPost, "/project/p1/service/svc1/auth"},
		{http.MethodPost, "/project/p1/billing/bp1"},
		// neighbours that must keep winning over the new params:
		{http.MethodGet, "/project/p1/service/details"},
		{http.MethodGet, "/project/p1/service/CLOUD/location"},
		{http.MethodGet, "/project/p1/billing"},
	} {
		rctx := chi.NewRouteContext()
		if !r.Match(rctx, tc.method, tc.path) {
			t.Errorf("no route for %s %s", tc.method, tc.path)
		}
	}
}
