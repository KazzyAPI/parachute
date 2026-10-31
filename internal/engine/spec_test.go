package engine

import (
	"testing"
	"time"

	"github.com/moby/moby/api/types/container"
)

func TestStopParamsLabelSignalAndContainerTimeoutWin(t *testing.T) {
	thirty := 30
	spec := Spec{Config: &container.Config{Labels: map[string]string{"parachute.stop-signal": "SIGHUP"}, StopTimeout: &thirty}}
	if signal, timeout := spec.StopParams(10 * time.Second); signal != "SIGHUP" || timeout != 30*time.Second {
		t.Fatalf("signal %q timeout %s", signal, timeout)
	}
	if signal, timeout := (Spec{}).StopParams(10 * time.Second); signal != "" || timeout != 10*time.Second {
		t.Fatalf("signal %q timeout %s", signal, timeout)
	}
}
