package verify

import (
	"testing"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
)

func TestEffectiveDeadline_without_health_uses_timeout(t *testing.T) {
	got := EffectiveDeadline(120*time.Second, nil)
	if got != 120*time.Second {
		t.Fatalf("got %s", got)
	}
}

func TestEffectiveDeadline_health_extends_past_timeout(t *testing.T) {
	health := &dockerspec.HealthcheckConfig{
		Test:        []string{"CMD", "true"},
		StartPeriod: 200 * time.Second,
		Interval:    30 * time.Second,
		Retries:     3,
	}
	got := EffectiveDeadline(120*time.Second, health)
	want := 200*time.Second + 30*time.Second*4
	if got != want {
		t.Fatalf("got %s want %s", got, want)
	}
}

func TestEffectiveDeadline_label_timeout_when_larger(t *testing.T) {
	health := &dockerspec.HealthcheckConfig{
		Test:     []string{"CMD", "true"},
		Interval: 2 * time.Second,
		Retries:  3,
	}
	got := EffectiveDeadline(20*time.Second, health)
	if got != 20*time.Second {
		t.Fatalf("got %s", got)
	}
}
