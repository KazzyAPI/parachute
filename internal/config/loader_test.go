package config

import (
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
)

type fakeSystem struct{}

func (fakeSystem) Hostname() (string, error) { return "box", nil }

func (fakeSystem) TempDir() string { return "/tmp" }

func (fakeSystem) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }

func (fakeSystem) Stat(path string) (fs.FileInfo, error) { return os.Stat(path) }

func newTestLoader() *Loader {
	registry := NewRegistry(DefaultSettings())
	removed, suggester, syntax := NewRemovedSettings(), NewSuggester(3), NewSyntax()
	return NewLoader(registry, NewArgumentParser(registry, removed, suggester), NewEnvironmentReader(registry, removed, suggester),
		NewResolver(registry, syntax, fakeSystem{}), fakeSystem{})
}

func TestListSyntaxAcceptsQuotesCommasAndSpaces(t *testing.T) {
	for raw, want := range map[string][]string{
		`"a, b c"`: {"a", "b", "c"},
		`'a,b'`:    {"a", "b"},
		"a  b,,c":  {"a", "b", "c"},
		"":         nil,
	} {
		if got := NewSyntax().List(raw); !slices.Equal(got, want) {
			t.Errorf("List(%q) = %v, want %v", raw, got, want)
		}
	}
}

func TestFlagWinsOverEnvironmentWinsOverDefault(t *testing.T) {
	cfg, err := newTestLoader().Load([]string{"--interval", "120"}, []string{"PARACHUTE_INTERVAL=60", "PARACHUTE_STATE_DIR=/s"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Schedule.Interval.Seconds() != 120 || cfg.StateDir != "/s" || cfg.Notify.Level != "info" {
		t.Errorf("unexpected config %+v", cfg)
	}
}

func TestConvenientProfileRelaxesOnlyImplicitDefaults(t *testing.T) {
	cfg, err := newTestLoader().Load([]string{"--profile", "convenient", "--armed=false", "--delay-days", "3"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cfg.Relaxed, []string{"protect-databases"}) || cfg.Armed {
		t.Errorf("relaxed %v armed %t", cfg.Relaxed, cfg.Armed)
	}
}

func TestFeatureSectionsAreBoundFromTypedValues(t *testing.T) {
	cfg, err := newTestLoader().Load([]string{"--delay-days", "0.5", "--maintenance-window-duration", "90m", "--http-api-port", "9000"},
		[]string{"PARACHUTE_HTTP_API_TOKEN=tok", "PARACHUTE_APPROVAL_REMINDER_DAYS=3"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Policy.DelayDays != 0.5 || cfg.Policy.WindowDuration.Minutes() != 90 || cfg.Policy.ReminderDays != 3 || !cfg.Policy.ProtectDatabases {
		t.Errorf("unexpected policy %+v", cfg.Policy)
	}
	if cfg.API.Port != 9000 || cfg.API.Token != "tok" || !cfg.Notify.Report || cfg.Engine.AllowInsecureTCP {
		t.Errorf("unexpected api %+v notify %+v", cfg.API, cfg.Notify)
	}
}

func TestStartupErrorsNameTheSetting(t *testing.T) {
	for _, tc := range []struct {
		args    []string
		environ []string
		want    []string
	}{
		{args: []string{"--intervall", "5"}, want: []string{"--intervall", "--interval"}},
		{environ: []string{"PARACHUTE_CLEANUP=yes"}, want: []string{"cleanup", "yes"}},
		{environ: []string{"PARACHUTE_NOTIFICATION_SLACK_HOOK_URL=x"}, want: []string{"--notification-url", "slack://"}},
		{args: []string{"--revive-stopped"}, want: []string{"revive-stopped", "include-stopped"}},
		{args: []string{"--interval", "5", "--schedule", "* * * * * *"}, want: []string{"interval", "schedule"}},
		{args: []string{"--armed"}, want: []string{"updates are not implemented in this milestone"}},
		{args: []string{"--http-api-status"}, want: []string{"--http-api-status", "not implemented"}},
		{args: []string{"--allow-insecure-tcp"}, want: []string{"--allow-insecure-tcp", "not implemented"}},
		{environ: []string{"PARACHUTE_TRUSTED_PROXIES=10.0.0.1"}, want: []string{"--trusted-proxies", "not implemented"}},
	} {
		_, err := newTestLoader().Load(tc.args, tc.environ)
		if err == nil {
			t.Errorf("%v %v: no error", tc.args, tc.environ)
			continue
		}
		for _, part := range tc.want {
			if !strings.Contains(err.Error(), part) {
				t.Errorf("%v %v: %q does not contain %q", tc.args, tc.environ, err, part)
			}
		}
	}
}
