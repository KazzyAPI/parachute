package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const containerIDPrefixLength = 12

type ContainerLister interface {
	Containers(ctx context.Context) ([]domain.Container, error)
}

type LabelReader interface {
	Bool(container domain.Container, name string) (bool, bool)
}

type Hostnamer interface {
	Hostname() (string, error)
}

type ArmedLabelGuard struct {
	engine   ContainerLister
	hostname Hostnamer
	labels   LabelReader
}

func NewArmedLabelGuard(engine ContainerLister, hostname Hostnamer, labels LabelReader) *ArmedLabelGuard {
	return &ArmedLabelGuard{engine: engine, hostname: hostname, labels: labels}
}

func (g *ArmedLabelGuard) Check(ctx context.Context) error {
	hostname, err := g.hostname.Hostname()
	if err != nil {
		return fmt.Errorf("cannot determine own hostname: %w", err)
	}
	if len(hostname) < containerIDPrefixLength {
		return nil
	}
	containers, err := g.engine.Containers(ctx)
	if err != nil {
		return unreachableError(err)
	}
	for _, container := range containers {
		if armed, _ := g.labels.Bool(container, "armed"); armed && strings.HasPrefix(container.ID, hostname) {
			return configurationError(errors.New("updates are not implemented in this milestone (parachute.armed=true on own container)"))
		}
	}
	return nil
}
