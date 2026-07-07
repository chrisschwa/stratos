package mail

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBuildMessage(t *testing.T) {
	// text-only
	b := string(buildMessage("a@x.io", Message{To: []string{"b@y.io"}, Subject: "Hi", TextBody: "hello"}))
	if !strings.Contains(b, "From: a@x.io") || !strings.Contains(b, "To: b@y.io") || !strings.Contains(b, "Subject: Hi") {
		t.Fatalf("missing headers:\n%s", b)
	}
	if !strings.Contains(b, "Content-Type: text/plain; charset=UTF-8") || !strings.Contains(b, "hello") {
		t.Fatalf("text body wrong:\n%s", b)
	}
	// html+text → multipart/alternative
	mb := string(buildMessage("a@x.io", Message{To: []string{"b@y.io"}, Subject: "S", TextBody: "t", HTMLBody: "<b>h</b>"}))
	if !strings.Contains(mb, "multipart/alternative") || !strings.Contains(mb, "text/html") || !strings.Contains(mb, "<b>h</b>") {
		t.Fatalf("multipart wrong:\n%s", mb)
	}
}

func TestFromEnvNoopWhenUnconfigured(t *testing.T) {
	t.Setenv("STRATOS_MAIL_SMTP_HOST", "")
	t.Setenv("STRATOS_MAIL_FROM", "")
	if _, ok := FromEnv().(NoopMailer); !ok {
		t.Fatal("expected NoopMailer when unconfigured")
	}
}

// TestSMTPSendSmoke is a LIVE send vs a real SMTP relay, gated on STRATOS_MAIL_SMTP_HOST so normal
// `go test ./...` never sends. Run:
//
//	STRATOS_MAIL_SMTP_HOST=smtp.sendgrid.net STRATOS_MAIL_SMTP_USERNAME=apikey \
//	STRATOS_MAIL_SMTP_PASSWORD=SG.xxx STRATOS_MAIL_FROM=hien@menlo.ai \
//	STRATOS_MAIL_TEST_TO=hien@menlo.ai go test ./internal/platform/mail -run TestSMTPSendSmoke -v
func TestSMTPSendSmoke(t *testing.T) {
	if os.Getenv("STRATOS_MAIL_SMTP_HOST") == "" {
		t.Skip("STRATOS_MAIL_SMTP_HOST not set — skipping live mail smoke")
	}
	to := os.Getenv("STRATOS_MAIL_TEST_TO")
	if to == "" {
		to = os.Getenv("STRATOS_MAIL_FROM")
	}
	m := FromEnv()
	if _, ok := m.(NoopMailer); ok {
		t.Fatal("mail env incomplete (need HOST + FROM)")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	err := m.Send(ctx, Message{
		To:       []string{to},
		Subject:  "Stratos Go — mail package live smoke",
		TextBody: "Sent by internal/platform/mail SMTPMailer at " + time.Now().UTC().Format(time.RFC1123Z) + ".",
		HTMLBody: "<p>Sent by <code>internal/platform/mail</code> SMTPMailer.</p>",
	})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	t.Logf("sent live test mail to %s", to)
}
