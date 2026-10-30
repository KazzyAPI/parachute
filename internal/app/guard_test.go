package app

import (
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
)

type fakeSelf struct{ found bool }

func (f fakeSelf) Mark(hosts []engine.SelfHost) bool {
	if f.found {
		hosts[0].Containers[0].Own = true
	}
	return f.found
}

type fakeLabels struct{ armed bool }

func (f fakeLabels) Is(_ domain.Container, name string) bool { return f.armed && name == "armed" }

func TestOwnContainerArms(t *testing.T) {
	cases := []struct {
		name   string
		self   fakeSelf
		labels fakeLabels
		want   bool
	}{
		{"own container labelled", fakeSelf{true}, fakeLabels{true}, true},
		{"own container unlabelled", fakeSelf{true}, fakeLabels{false}, false},
		{"own container not found", fakeSelf{false}, fakeLabels{true}, false},
	}
	for _, c := range cases {
		hosts := []engine.SelfHost{{Local: true, Containers: []domain.Container{{ID: "a"}}}}
		if got := ownContainerArms(c.self, c.labels, hosts); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
