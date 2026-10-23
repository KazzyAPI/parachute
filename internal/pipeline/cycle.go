package pipeline

import (
	"context"
	"log/slog"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/registry"
)

type ContainerSource interface {
	Containers(ctx context.Context) ([]domain.Container, error)
	Image(ctx context.Context, reference string) (engine.Image, bool, error)
}

type ResultDetector interface {
	Detect(ctx context.Context, result domain.Result, credentials registry.Credentials, noPull bool) (domain.Result, registry.Helper, error)
}

type Host struct {
	Name     string
	Endpoint string
	Source   ContainerSource
	Detector ResultDetector
}

type HostRun struct {
	Host       Host
	Reachable  bool
	Containers []domain.Container
	Candidates []*Candidate
}

type Candidate struct {
	Container domain.Container
	Result    domain.Result
}

type Cycle struct {
	ID      string
	Started time.Time
	Armed   bool
	Hosts   []*HostRun
	Report  domain.Report
}

func NewCycle(id string, started time.Time, armed bool, hosts []Host) *Cycle {
	cycle := &Cycle{ID: id, Started: started, Armed: armed}
	for _, host := range hosts {
		cycle.Hosts = append(cycle.Hosts, &HostRun{Host: host, Reachable: true})
	}
	return cycle
}

func (h *HostRun) Unreachable(logger *slog.Logger, cycleID string, err error) {
	h.Reachable = false
	logger.Error("host unreachable", "cycle", cycleID, "host", h.Host.Name, "endpoint", h.Host.Endpoint, "error", err.Error())
}

func (c *Cycle) ReachableHosts() []*HostRun {
	var reachable []*HostRun
	for _, host := range c.Hosts {
		if host.Reachable {
			reachable = append(reachable, host)
		}
	}
	return reachable
}

type Stage interface {
	Run(ctx context.Context, cycle *Cycle) error
}

type Pipeline struct {
	stages []Stage
}

func New(stages ...Stage) *Pipeline {
	return &Pipeline{stages: stages}
}

func (p *Pipeline) Run(ctx context.Context, cycle *Cycle) error {
	for _, stage := range p.stages {
		if err := stage.Run(ctx, cycle); err != nil {
			return err
		}
	}
	return nil
}
