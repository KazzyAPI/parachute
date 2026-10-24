package app

import (
	"testing"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/labels"
)

func TestContainer_verifier_returns_interface(t *testing.T) {
	container := NewContainer(config.Config{Verify: config.Verify{Timeout: 120, Stable: 30}}, Dependencies{}, nil, nil, nil)
	container.labels = labels.NewReader(nil)
	verifier := container.verifier(nil)
	if verifier == nil {
		t.Fatal("expected verifier")
	}
}
