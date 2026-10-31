package engine

import (
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
)

type Spec struct {
	ID           string
	Name         string
	ImageID      string
	HealthStatus string
	Status       string
	ExitCode     int
	Config       *container.Config
	HostConfig   *container.HostConfig
	Networks     map[string]*network.EndpointSettings
	Mounts       []container.MountPoint
	ImageConfig  *dockerspec.DockerOCIImageConfig
}

type RegistryAuth string

type ExecResult struct {
	ExitCode  int
	Output    []byte
	Stderr    []byte
	Truncated bool
}

func (r ExecResult) Combined() []byte {
	return append(append([]byte{}, r.Output...), r.Stderr...)
}

type Event struct {
	Type    string
	Action  string
	ActorID string
	Time    time.Time
}
