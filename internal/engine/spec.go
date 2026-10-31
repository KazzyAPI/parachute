package engine

import (
	"strings"
	"time"

	dockerspec "github.com/moby/docker-image-spec/specs-go/v1"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"github.com/KazzyAPI/parachute/internal/redact"
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

func (s Spec) RedactEnv(text string) string {
	var values []string
	if s.Config != nil {
		for _, pair := range s.Config.Env {
			if _, value, ok := strings.Cut(pair, "="); ok {
				values = append(values, value)
			}
		}
	}
	return redact.New(redact.Static(values...)).Redact(text)
}

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
