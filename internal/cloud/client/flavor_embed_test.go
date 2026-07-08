package client

import "testing"

// nova ≥ mv2.47 embeds the flavor specs (no id) — the shape the sync must read so rating isn't zero.
func TestFillEmbeddedFlavor_Embedded(t *testing.T) {
	srv := &Server{}
	fillEmbeddedFlavor(srv, map[string]any{
		"original_name": "gpu.8xa6000",
		"ram":           float64(131072), // JSON numbers decode to float64
		"vcpus":         float64(32),
		"disk":          float64(400),
		"extra_specs":   map[string]any{"pci_passthrough:alias": "a6000:8"},
	})
	if srv.FlavorName != "gpu.8xa6000" || srv.RAM != 131072 || srv.VCPUs != 32 || srv.Disk != 400 {
		t.Fatalf("embedded specs not read: %+v", srv)
	}
	if srv.FlavorExtraSpecs["pci_passthrough:alias"] != "a6000:8" {
		t.Errorf("extra_specs not read: %v", srv.FlavorExtraSpecs)
	}
}

// A bare {id,links} link (older microversion) → no-op, so the caller falls back to by-id resolution.
func TestFillEmbeddedFlavor_BareLink(t *testing.T) {
	srv := &Server{}
	fillEmbeddedFlavor(srv, map[string]any{"id": "f-1", "links": []any{}})
	if srv.RAM != 0 || srv.VCPUs != 0 || srv.FlavorName != "" {
		t.Errorf("bare link should not populate specs: %+v", srv)
	}
}
