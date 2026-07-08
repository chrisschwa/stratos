package cloud

import (
	"strconv"
	"strings"
)

// GPUFromFlavor derives the GPU model alias + device count from a nova flavor's
// extra specs (values arrive as any JSON scalar after the cloudResource cache round-trip).
//
// Primary source: `pci_passthrough:alias` = "<alias>:<count>[,<alias>:<count>…]" —
// model = first alias, count = total across entries.
// Fallback: `resources:VGPU` = "<count>" → model "vgpu".
//
// The alias is normalized (lowercase, "_"→"-") to match the placement-trait vocabulary
// (CUSTOM_PCI_A100_80GB → "a100-80gb") shared by gpu-info capacity and project GPU quota,
// so pricing rules, capacity and quota all key on one model name.
func GPUFromFlavor(extraSpecs map[string]any) (model string, count int) {
	if extraSpecs == nil {
		return "", 0
	}
	if alias, ok := extraSpecs["pci_passthrough:alias"].(string); ok && strings.TrimSpace(alias) != "" {
		for _, part := range strings.Split(alias, ",") {
			name, n, ok := strings.Cut(strings.TrimSpace(part), ":")
			if !ok {
				continue
			}
			c, err := strconv.Atoi(strings.TrimSpace(n))
			if err != nil || c <= 0 {
				continue
			}
			if model == "" {
				model = NormalizeGPUAlias(name)
			}
			count += c
		}
		if count > 0 {
			return model, count
		}
	}
	if vgpu, ok := extraSpecs["resources:VGPU"].(string); ok {
		if c, err := strconv.Atoi(strings.TrimSpace(vgpu)); err == nil && c > 0 {
			return "vgpu", c
		}
	}
	return "", 0
}

// NormalizeGPUAlias lowercases and dash-normalizes a GPU alias / trait suffix so
// "A100_80GB" (from trait CUSTOM_PCI_A100_80GB) and "a100-80gb" (pci alias) compare equal.
func NormalizeGPUAlias(s string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(s)), "_", "-")
}

// GPUFromSpecStrings adapts a live (typed) flavor extra-specs map to GPUFromFlavor —
// the cloud client returns map[string]string, the cached docs map[string]any.
func GPUFromSpecStrings(specs map[string]string) (model string, count int) {
	m := make(map[string]any, len(specs))
	for k, v := range specs {
		m[k] = v
	}
	return GPUFromFlavor(m)
}
