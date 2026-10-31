package config

import "testing"

func TestLoadRejectsOutOfRangeAndEmptyFlagValues(t *testing.T) {
	cases := map[string][]string{
		"port":          {"--http-api-update", "--http-api-port", "70000", "--http-api-token", "t"},
		"interval":      {"--interval", "9223372037"},
		"delay-days":    {"--delay-days", "200000"},
		"empty porcel.": {"--porcelain", ""},
		"empty sched.":  {"--schedule", ""},
	}
	for name, args := range cases {
		if _, err := newTestLoader().Load(args, nil); err == nil {
			t.Errorf("%s: expected a startup error", name)
		}
	}
}
