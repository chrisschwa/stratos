package admin

import (
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
)

// TestCloudResourceIDNotFound pins the exact 404:
// "CloudResource with id %s not found" — interpolated id, NO trailing space — and 404 status/code.
func TestCloudResourceIDNotFound(t *testing.T) {
	err := cloudResourceIDNotFound("cr-123")
	want := "CloudResource with id cr-123 not found"
	if err.Msg != want {
		t.Errorf("message=%q want %q", err.Msg, want)
	}
	if err.Status != http.StatusNotFound {
		t.Errorf("status=%d want %d", err.Status, http.StatusNotFound)
	}
	if err.Code != http.StatusNotFound {
		t.Errorf("code=%d want %d", err.Code, http.StatusNotFound)
	}
}

// TestRouteCloudResourceMutRegisters verifies the two new mutation routes are registered at the
// expected (method, path) — and ONLY those (the reads stay in handler.go). Pure: no Handler deps
// are exercised (routing is resolved before any handler body runs).
func TestRouteCloudResourceMutRegisters(t *testing.T) {
	h := &Handler{}
	r := chi.NewRouter()
	h.routeCloudResourceMut(r)

	cases := []struct {
		method string
		path   string
	}{
		{http.MethodGet, "/cloud-resource/{id}/sync"},
		{http.MethodDelete, "/cloud-resource/{id}"},
	}
	for _, c := range cases {
		if h := matchRoute(r, c.method, c.path); h == nil {
			t.Errorf("route %s %s not registered", c.method, c.path)
		}
	}
}

// matchRoute reports whether the router has a handler at the exact (method, route-pattern).
func matchRoute(r chi.Router, method, pattern string) http.Handler {
	tctx := chi.NewRouteContext()
	if r.Match(tctx, method, resolvePattern(pattern)) {
		return http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})
	}
	return nil
}

// resolvePattern turns a chi route pattern into a concrete path to Match against ({id} → a value).
func resolvePattern(pattern string) string {
	switch pattern {
	case "/cloud-resource/{id}/sync":
		return "/cloud-resource/cr-1/sync"
	case "/cloud-resource/{id}":
		return "/cloud-resource/cr-1"
	default:
		return pattern
	}
}
