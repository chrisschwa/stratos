package project

import "testing"

// Live zones as ListAvailabilityZones returns them.
func liveZones() []map[string]any {
	return []map[string]any{
		{"name": "nova", "available": true},
		{"name": "az-b", "available": true},
	}
}

func TestApplyZoneConfig_NoConfigPassesThrough(t *testing.T) {
	out := applyZoneConfig(liveZones(), nil)
	if len(out) != 2 {
		t.Fatalf("want 2 zones, got %d", len(out))
	}
	// No curation → zones pass through untouched (the FE renders z.displayName || z.name, so the
	// raw name still shows). Nothing is filtered or renamed.
	if out[0]["name"] != "nova" || out[1]["name"] != "az-b" {
		t.Errorf("zones mutated: %v", out)
	}
}

func TestApplyZoneConfig_DisplayNameAndFilter(t *testing.T) {
	cfg := []any{
		map[string]any{"name": "nova", "displayName": "az1", "enabled": true},
		map[string]any{"name": "az-b", "displayName": "", "enabled": false},
	}
	out := applyZoneConfig(liveZones(), cfg)
	if len(out) != 1 {
		t.Fatalf("disabled az-b must be dropped: got %d zones", len(out))
	}
	if out[0]["name"] != "nova" {
		t.Errorf("real name must stay nova, got %v", out[0]["name"])
	}
	if out[0]["displayName"] != "az1" {
		t.Errorf("displayName = %v, want az1", out[0]["displayName"])
	}
}

// A zone the admin never curated (config present but no entry) is kept with its raw name, so a
// newly-appeared cloud zone is never silently hidden.
func TestApplyZoneConfig_UnknownZoneKept(t *testing.T) {
	cfg := []any{map[string]any{"name": "nova", "displayName": "az1", "enabled": true}}
	out := applyZoneConfig(liveZones(), cfg)
	if len(out) != 2 {
		t.Fatalf("want 2 zones, got %d", len(out))
	}
	if out[1]["name"] != "az-b" || out[1]["displayName"] != "az-b" {
		t.Errorf("uncurated zone = %v, want name/displayName az-b", out[1])
	}
}

// The legacy name-keyed map shape must be honored, not treated as "no config".
func TestApplyZoneConfig_MapShape(t *testing.T) {
	cfg := map[string]any{
		"nova": map[string]any{"displayName": "az1", "enabled": true},
		"az-b": map[string]any{"displayName": "", "enabled": false},
	}
	out := applyZoneConfig(liveZones(), cfg)
	if len(out) != 1 || out[0]["name"] != "nova" || out[0]["displayName"] != "az1" {
		t.Fatalf("map shape not applied: %v", out)
	}
}

func TestHasEnabledZone(t *testing.T) {
	// No config → not curated.
	if en, cur := hasEnabledZone(nil); cur || en {
		t.Errorf("nil cfg: got enabled=%v curated=%v, want false/false", en, cur)
	}
	// Curated, all disabled → the state that must block server create.
	allOff := []any{
		map[string]any{"name": "nova", "enabled": false},
		map[string]any{"name": "az-b", "enabled": false},
	}
	if en, cur := hasEnabledZone(allOff); !cur || en {
		t.Errorf("all-off: got enabled=%v curated=%v, want false/true", en, cur)
	}
	// Curated with one enabled (map shape) → allowed.
	oneOn := map[string]any{"nova": map[string]any{"enabled": true}}
	if en, cur := hasEnabledZone(oneOn); !cur || !en {
		t.Errorf("one-on: got enabled=%v curated=%v, want true/true", en, cur)
	}
}
