package app

import (
	"bytes"
	"strings"
	"testing"
)

func TestInformationalArguments(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"-h"}, {"help"}} {
		print, ok := informational(args)
		if !ok {
			t.Fatalf("%v not informational", args)
		}
		var out bytes.Buffer
		if err := print(&out, "1.2.3"); err != nil || !strings.Contains(out.String(), "Usage:") {
			t.Errorf("%v: %q %v", args, out.String(), err)
		}
	}
	print, ok := informational([]string{"version"})
	if !ok {
		t.Fatal("version not informational")
	}
	var out bytes.Buffer
	if err := print(&out, "1.2.3"); err != nil || out.String() != "parachute 1.2.3\n" {
		t.Errorf("%q %v", out.String(), err)
	}
	if _, ok := informational([]string{"--armed"}); ok {
		t.Error("--armed treated as informational")
	}
}
