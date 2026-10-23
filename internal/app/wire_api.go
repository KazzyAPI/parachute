package app

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/schedule"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/status"
)

type Server interface {
	Serve(ctx context.Context) error
}

type ServerParts struct {
	Gate   *schedule.Gate
	Runner CycleRunner
	Store  *state.Store
	Status status.Source
	Hosts  []pipeline.Host
}

type idleServer struct{}

func (idleServer) Serve(context.Context) error { return nil }

func (c *Container) server(_ ServerParts) (Server, error) {
	return idleServer{}, nil
}
