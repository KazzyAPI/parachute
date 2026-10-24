package state

import (
	"slices"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type ServiceTransaction struct {
	Host        string `json:"host"`
	Container   string `json:"container"`
	Transaction string `json:"transaction"`
}

func (d Document) ActiveFreezes() []Freeze {
	var active []Freeze
	for _, freeze := range d.Freezes {
		released := slices.ContainsFunc(d.Releases, func(release Release) bool {
			return release.Transaction == freeze.Transaction && slices.Equal(release.Keys, freeze.Keys)
		})
		if !released {
			active = append(active, freeze)
		}
	}
	return active
}

func (f Freeze) HasStack(host, stack string) bool {
	return slices.Contains(f.Keys, FreezeKey{Host: host, Stack: stack})
}

func (f Freeze) Covers(host string, container domain.Container) bool {
	return f.HasStack(host, container.Stack()) || slices.Contains(f.Keys, FreezeKey{Host: host, Container: container.Name})
}

func (d Document) Failed(host, container, digest string) bool {
	return slices.Contains(d.FailedDigests, FailedDigest{Host: host, Container: container, Digest: digest})
}
