package redact

import (
	"encoding/base64"
	"strings"
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
		`{"Authorization":"Bearer tok"}`:                        `{"Authorization":"Bearer [REDACTED]"}`,
		`{"Authorization":"tok"}`:                               `{"Authorization":"[REDACTED]"}`,
		"got Basic dXNlcjpwYXNz= from proxy":                    "got Basic [REDACTED] from proxy",
		`header "Basic dXNlcg=="`:                               `header "Basic [REDACTED]"`,
		`www-authenticate: Bearer realm="https://a.test/token"`: `www-authenticate: Bearer realm="https://a.test/token"`,
		"bearer eyJhbGciOi.eyJzdWIi.sig expired":                "bearer [REDACTED] expired",
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

func TestURLSecretsCoverNotificationURLParts(t *testing.T) {
	redactor := New(URLSecrets([]string{
		"generic://bob:hunt%40er2@127.0.0.1:1/secret-path/Ab12cd34ef?token=q5%2Bcret&disabletls=true&port=8080&template=json&pin=1234567&scheme=https&proto=https",
		"ifttt://KeyOfIfttt/?events=doorbell",
		"telegram://telegram/webhook",
	}))
	for input, want := range map[string]string{
		"password hunt@er2 hunt%40er2":                        "password [REDACTED] [REDACTED]",
		"value q5+cret q5%2Bcret":                             "value [REDACTED] [REDACTED]",
		"Post http://127.0.0.1:1/secret-path/Ab12cd34ef: EOF": "Post http://[REDACTED]: EOF",
		"segment secret-path Ab12cd34ef, key KeyOfIfttt":      "segment secret-path [REDACTED], key [REDACTED]",
		"pull https://bob:regpass99@registry.example/v2":      "pull [REDACTED]://[REDACTED]@registry.example/v2",
		"telegram webhook services":                           "telegram webhook services",
		"pin 1234567 event doorbell":                          "pin [REDACTED] event [REDACTED]",
		"host 127.0.0.1:1 true 8080 json":                     "host 127.0.0.1:1 true 8080 json",
	} {
		if got := redactor.Redact(input); got != want {
			t.Errorf("Redact(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWriterRedactsWholeLines(t *testing.T) {
	var out strings.Builder
	writer := NewWriter(&out, New(Static("hunter22")))
	for _, part := range []string{"a hun", "ter22 b\nc ", "hunter22\n"} {
		if _, err := writer.Write([]byte(part)); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := writer.Write([]byte("tail hunter22")); err != nil || writer.Flush() != nil {
		t.Fatal("tail not written")
	}
	if want := "a [REDACTED] b\nc [REDACTED]\ntail [REDACTED]"; out.String() != want {
		t.Errorf("wrote %q, want %q", out.String(), want)
	}
}
