package restoreverify

import (
	"testing"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/container"
)

func TestSanitizeLabelsStripsParachuteAndCompose(t *testing.T) {
	source := map[string]string{
		"com.docker.compose.project": "shop",
		"parachute.transaction":      "txn-20260101T000000Z-abcdef",
		"parachute.helper":           "false",
		"parachute.restore-verify":   "rv-20260101T000000Z-abcdef",
		"parachute.verify.stable":    "5s",
	}
	got := SanitizeLabels(source, "rv-20260201T120000Z-fedcba")
	want := map[string]string{
		"parachute.helper":         "true",
		"parachute.restore-verify": "rv-20260201T120000Z-fedcba",
		"parachute.verify.stable":  "5s",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("label %s = %q, want %q", key, got[key], value)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("labels = %v, want %v", got, want)
	}
}

func TestThrowawaySpecHardensHostConfig(t *testing.T) {
	record := engine.Spec{
		Name: "db",
		Config: &container.Config{
			Image:  "busybox",
			Labels: map[string]string{"parachute.transaction": "txn-20260101T000000Z-abcdef"},
		},
		HostConfig: &container.HostConfig{
			Privileged:    true,
			NetworkMode:   "host",
			Binds:         []string{"/:/host"},
			RestartPolicy: container.RestartPolicy{Name: "always"},
		},
	}
	spec := ThrowawaySpec(record, "rv-20260201T120000Z-fedcba", "fedcba", "parachute-rv-fedcba", "db", nil)
	if spec.HostConfig.Privileged {
		t.Fatal("privileged")
	}
	if spec.HostConfig.NetworkMode == "host" {
		t.Fatal("host network")
	}
	if len(spec.HostConfig.Binds) > 0 {
		t.Fatal("binds")
	}
	if spec.HostConfig.RestartPolicy.Name != "no" {
		t.Fatal("restart policy")
	}
}

func TestNamesAndSuffix(t *testing.T) {
	runID := "rv-20260201T120000Z-fedcba"
	if Suffix(runID) != "fedcba" {
		t.Fatalf("suffix %q", Suffix(runID))
	}
	if ThrowawayContainerName(Suffix(runID), "web") != "parachute-rv-fedcba-web" {
		t.Fatal("container name")
	}
	if ThrowawayNetworkName(Suffix(runID)) != "parachute-rv-fedcba" {
		t.Fatal("network name")
	}
}

func TestRemovableRequiresHelperAndRunID(t *testing.T) {
	allowed := map[string]bool{"rv-1": true}
	if Removable(map[string]string{"parachute.helper": "true", "parachute.restore-verify": "rv-1"}, allowed) != true {
		t.Fatal("expected removable")
	}
	if Removable(map[string]string{}, allowed) {
		t.Fatal("empty labels")
	}
	if Removable(map[string]string{"parachute.helper": "true"}, allowed) {
		t.Fatal("missing run id")
	}
	if Removable(map[string]string{"parachute.helper": "true", "parachute.restore-verify": "rv-2"}, allowed) {
		t.Fatal("wrong run id")
	}
}

func TestNamePrefixAloneNeverRemovable(t *testing.T) {
	allowed := map[string]bool{"rv-1": true}
	if Removable(map[string]string{"name": "parachute-rv-abcdef-keep"}, allowed) {
		t.Fatal("name prefix must not remove")
	}
}
