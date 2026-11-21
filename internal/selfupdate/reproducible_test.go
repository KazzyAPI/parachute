package selfupdate

import (
	"net/netip"
	"testing"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

func TestSideBySideFieldRejectsPublishedPort(t *testing.T) {
	spec := engine.Spec{HostConfig: &container.HostConfig{PortBindings: map[network.Port][]network.PortBinding{
		network.MustParsePort("8080/tcp"): {{HostPort: "8080"}},
	}}}
	if field, ok := SideBySideField(spec); !ok || field != "8080:8080" {
		t.Fatalf("field %q ok %v", field, ok)
	}
}

func TestSideBySideFieldRejectsStaticIP(t *testing.T) {
	spec := engine.Spec{Networks: map[string]*network.EndpointSettings{"net": {IPAMConfig: &network.EndpointIPAMConfig{IPv4Address: netip.MustParseAddr("10.0.0.2")}}}}
	if field, ok := SideBySideField(spec); !ok || field == "" {
		t.Fatalf("field %q ok %v", field, ok)
	}
}

func TestSideBySideFieldAllowsPlainContainer(t *testing.T) {
	if field, ok := SideBySideField(engine.Spec{}); ok || field != "" {
		t.Fatalf("field %q ok %v", field, ok)
	}
}

func TestSideBySideFieldRejectsEphemeralPublishedPort(t *testing.T) {
	spec := engine.Spec{HostConfig: &container.HostConfig{PortBindings: map[network.Port][]network.PortBinding{
		network.MustParsePort("8080/tcp"): {{}},
	}}}
	if field, ok := SideBySideField(spec); !ok || field != "published port 8080" {
		t.Fatalf("field %q ok %v", field, ok)
	}
}

func TestSideBySideFieldAllowsEngineAssignedAddress(t *testing.T) {
	spec := engine.Spec{Networks: map[string]*network.EndpointSettings{"bridge": {IPAddress: netip.MustParseAddr("172.17.0.2")}}}
	if field, ok := SideBySideField(spec); ok {
		t.Fatalf("field %q", field)
	}
}
