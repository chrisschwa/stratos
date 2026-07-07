package livechat

import (
	"testing"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// TestInstallationCodeDTOStripsSecret is the regression guard for finding [26]: the
// installation-code read must expose ONLY the public embed script, never the raw integration
// row (which carries the integration's secret/config credentials).
func TestInstallationCodeDTOStripsSecret(t *testing.T) {
	row := pgdoc.M{
		"_id":      "lc-1",
		"category": "LiveChat",
		"secret":   pgdoc.M{"apiKey": "super-secret"},
		"config":   pgdoc.M{"liveChatScript": "<script>embed</script>", "accountToken": "tok"},
	}
	dto := installationCodeDTO(row)

	if dto["installationCode"] != "<script>embed</script>" {
		t.Fatalf("installationCode=%#v want the embed script", dto["installationCode"])
	}
	// No credential-bearing keys may survive into the response.
	for _, k := range []string{"secret", "config", "_id", "category"} {
		if _, ok := dto[k]; ok {
			t.Errorf("key %q must NOT leak into installation-code DTO, dto=%#v", k, dto)
		}
	}
}
