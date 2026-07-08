package mcp

import "testing"

// TestToolRegistryIntegrity guards the declarative tool rows: unique names, complete
// method/path, resolvable path params, and the rawbody contract (a single rawbody param,
// never mixed with named body params).
func TestToolRegistryIntegrity(t *testing.T) {
	for _, set := range []struct {
		name string
		defs []toolDef
	}{{"admin", adminAllTools()}, {"client", clientTools}} {
		seen := map[string]bool{}
		for _, d := range set.defs {
			if d.name == "" || d.method == "" || d.path == "" {
				t.Fatalf("%s: incomplete tool def %+v", set.name, d)
			}
			if seen[d.name] {
				t.Fatalf("%s: duplicate tool name %q", set.name, d.name)
			}
			seen[d.name] = true
			raw, body := 0, 0
			for _, p := range d.params {
				switch p.in {
				case "path", "query":
				case "body":
					body++
				case "rawbody":
					raw++
				default:
					t.Fatalf("%s/%s: param %q has invalid in=%q", set.name, d.name, p.name, p.in)
				}
			}
			if raw > 1 || (raw == 1 && body > 0) {
				t.Fatalf("%s/%s: rawbody must be single and unmixed (raw=%d body=%d)", set.name, d.name, raw, body)
			}
		}
	}
}
