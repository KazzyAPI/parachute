package engine

import "testing"

func TestHelperConfigClearsImageEntrypoint(t *testing.T) {
	config, hostConfig := helperConfig(HelperSpec{Image: "parachute", Command: []string{"sh", "-c", "true"}})
	if len(config.Entrypoint) != 1 || config.Entrypoint[0] != "" {
		t.Fatalf("entrypoint %q does not reset the image entrypoint", config.Entrypoint)
	}
	if !hostConfig.ReadonlyRootfs || string(hostConfig.NetworkMode) != "default" {
		t.Fatalf("unexpected host config %+v", hostConfig)
	}
}
