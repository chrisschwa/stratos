// Package livechat serves the LiveChat installation-code read. It looks for a configured
// "LiveChat" thirdPartyIntegration whose config carries a non-empty liveChatScript; with none
// (the greenfield seed) it returns the empty envelope {}.
//
// EMPTY-STATE: when an integration IS present, a fresh script is generated via the live-chat
// client (Chatwoot/Crisp/Intercom — a gated marketing integration) and wrapped in an
// installation-code shape. That populated shaping is deferred; the passthrough here would not
// match, so a populated deployment fails loudly (billing-list precedent).
package livechat

import (
	"context"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/menlocloud/stratos/internal/pgdoc"
	"github.com/menlocloud/stratos/pkg/httpx"
)

type Repo struct{ integrations *pgdoc.Store }

func NewRepo(db *pgdoc.DB) *Repo {
	return &Repo{integrations: db.C("thirdPartyIntegration")}
}

// LiveChatIntegration finds the first "LiveChat" integration with a non-empty
// config.liveChatScript; nil when none.
func (r *Repo) LiveChatIntegration(ctx context.Context) (pgdoc.M, error) {
	var rows []pgdoc.M
	if err := r.integrations.Find(ctx, pgdoc.M{"category": "LiveChat"}, &rows); err != nil {
		return nil, err
	}
	for _, row := range rows {
		if cfg, ok := row["config"].(map[string]any); ok {
			if s, _ := cfg["liveChatScript"].(string); s != "" {
				return row, nil
			}
		}
	}
	return nil, nil
}

type Handler struct{ repo *Repo }

func NewHandler(repo *Repo) *Handler { return &Handler{repo: repo} }

func (h *Handler) Routes(r chi.Router) {
	r.Get("/live-chat/installation-code", h.installationCode)
}

func (h *Handler) installationCode(w http.ResponseWriter, r *http.Request) {
	integration, err := h.repo.LiveChatIntegration(r.Context())
	if httpx.WriteError(w, err) {
		return
	}
	if integration == nil {
		httpx.Empty(w) // empty envelope → {}
		return
	}
	// Return ONLY the public embed script — never the raw integration row, which carries the
	// integration's secret/config credentials.
	httpx.OK(w, installationCodeDTO(integration))
}

// installationCodeDTO projects the integration row to the public installation-code shape:
// just the embed script from config.liveChatScript. Nothing else from the row (secret/config
// credentials) is exposed.
func installationCodeDTO(integration pgdoc.M) pgdoc.M {
	var script string
	if cfg, ok := integration["config"].(map[string]any); ok {
		script, _ = cfg["liveChatScript"].(string)
	}
	return pgdoc.M{"installationCode": script}
}
