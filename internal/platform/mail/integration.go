package mail

import (
	"context"
	"strconv"

	"github.com/menlocloud/stratos/internal/pgdoc"
)

// integration.go builds a Mailer from the admin-configured "SMTP" thirdPartyIntegration (Admin →
// Integrations → Mail). This is the DB mail gateway an operator sets up in the UI; it takes
// precedence over the STRATOS_MAIL_* env gateway (which stays as the bootstrap/fallback).

// smtpIntegration decodes the "SMTP" thirdPartyIntegration doc. Field keys match the admin
// integrations page SMTP schema (web/admin IntegrationsPage): config.{domain,port,username,
// fromName,fromEmail,noAuth} + secret.password. The secret is stored as-provided (the at-rest
// encryptor is not wired), so no decrypt step is needed here.
type smtpIntegration struct {
	Config struct {
		Domain    string `json:"domain"`
		Port      any    `json:"port"`
		Username  string `json:"username"`
		FromName  string `json:"fromName"`
		FromEmail string `json:"fromEmail"`
		NoAuth    bool   `json:"noAuth"`
	} `json:"config"`
	Secret struct {
		Password string `json:"password"`
	} `json:"secret"`
}

// SMTPFromStore builds an SMTP Mailer from the "SMTP" thirdPartyIntegration in the given store.
// Returns ok=false when no SMTP integration is configured, or it lacks host/from — the caller then
// falls back to the STRATOS_MAIL_* env gateway (FromEnv).
func SMTPFromStore(ctx context.Context, store *pgdoc.Store) (Mailer, string, bool) {
	if store == nil {
		return nil, "", false
	}
	var doc smtpIntegration
	ok, err := store.FindOne(ctx, pgdoc.M{"thirdParty": "SMTP"}, &doc)
	if err != nil || !ok {
		return nil, "", false
	}
	c := doc.Config
	if c.Domain == "" || c.FromEmail == "" { // mirror Config.Enabled(): need host + from
		return nil, "", false
	}
	cfg := Config{
		Host:     c.Domain,
		Port:     portToStr(c.Port),
		Username: c.Username,
		Password: doc.Secret.Password,
		From:     c.FromEmail,
	}
	if c.NoAuth {
		cfg.Username, cfg.Password = "", ""
	}
	from := c.FromEmail
	if c.FromName != "" {
		from = c.FromName + " <" + c.FromEmail + ">"
	}
	return NewSMTPMailer(cfg), from, true
}

// portToStr renders the JSON-decoded port (JSON yields float64/int32/int64/string) as a
// string, defaulting to 587 when absent or zero.
func portToStr(v any) string {
	switch n := v.(type) {
	case string:
		if n != "" {
			return n
		}
	case float64:
		if n != 0 {
			return strconv.FormatInt(int64(n), 10)
		}
	case int32:
		if n != 0 {
			return strconv.FormatInt(int64(n), 10)
		}
	case int64:
		if n != 0 {
			return strconv.FormatInt(n, 10)
		}
	case int:
		if n != 0 {
			return strconv.Itoa(n)
		}
	}
	return "587"
}
