package verify

import (
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
)

func EffectiveDeadline(timeout time.Duration, health *dockerspec.HealthcheckConfig) time.Duration {
	if health == nil || len(health.Test) == 0 || health.Test[0] == "NONE" {
		return timeout
	}
	interval := health.Interval
	if interval == 0 {
		interval = 30 * time.Second
	}
	retries := health.Retries
	if retries == 0 {
		retries = 3
	}
	minimum := health.StartPeriod + interval*time.Duration(retries+1)
	if minimum > timeout {
		return minimum
	}
	return timeout
}
