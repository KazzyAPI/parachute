package selfupdate

import (
	"fmt"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/recreate"
)

func SideBySideField(spec engine.Spec) (string, bool) {
	if spec.HostConfig != nil {
		for containerPort, bindings := range spec.HostConfig.PortBindings {
			for _, binding := range bindings {
				if binding.HostPort != "" {
					return fmt.Sprintf("%s:%d", binding.HostPort, containerPort.Num()), true
				}
			}
		}
	}
	for name, settings := range spec.Networks {
		if settings != nil && settings.IPAddress.IsValid() {
			if name == "" {
				return "IPAddress", true
			}
			return fmt.Sprintf("network %s IPAddress", name), true
		}
	}
	return "", false
}

func SideBySideFailure(spec engine.Spec) error {
	if field, blocked := SideBySideField(spec); blocked {
		return recreate.Failure{State: "failed", Reason: "config_unreproducible", Detail: field}
	}
	return nil
}
