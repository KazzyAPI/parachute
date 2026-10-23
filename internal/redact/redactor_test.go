package redact

import (
	"encoding/base64"
	"testing"
)

func TestRedactorReplacesSecretsAndURLCredentials(t *testing.T) {
	redactor := New(Static("T0ken-api", "user:s3cret-value", "ab"))
	encoded := base64.StdEncoding.EncodeToString([]byte("user:s3cret-value"))
	for input, want := range map[string]string{
		"token T0ken-api sent": "token [REDACTED] sent",
		"auth " + encoded:      "auth [REDACTED]",
		"generic://bob:hunter2@127.0.0.1:1/hook?token=q5ecret":  "generic://[REDACTED]@127.0.0.1:1/hook?token=[REDACTED]",
		`Get "https://reg.test/token?scope=a&service=b": EOF`:   `Get "https://reg.test/token?scope=[REDACTED]&service=[REDACTED]": EOF`,
		"Authorization: Bearer abc.def":                         "Authorization: Bearer [REDACTED]",
		"unix:///var/run/docker.sock and tab":                   "unix:///var/run/docker.sock and tab",
		"https://user@host/path#frag mentions ab and an e-mail": "https://[REDACTED]@host/path#frag mentions ab and an e-mail",
	} {
		if got := redactor.Redact(input); got != want {
			t.Errorf("Redact(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRedactorReadsSourcesOnEveryCall(t *testing.T) {
	source := &growing{}
	redactor := New(source)
	source.values = append(source.values, "late-secret")
	if got := redactor.Redact("a late-secret b"); got != "a [REDACTED] b" {
		t.Errorf("got %q", got)
	}
}

type growing struct{ values []string }

func (g *growing) Secrets() []string { return g.values }
