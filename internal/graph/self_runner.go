package graph

import (
	"context"

	"github.com/KazzyAPI/parachute/internal/pipeline"
)

type HandoverTicket struct {
	RemoveMarker bool
}

type SelfRunner interface {
	Handover(ctx context.Context, host string, own *pipeline.Candidate) (HandoverTicket, error)
	Finish(ctx context.Context, ticket HandoverTicket) error
}

type NoSelfRunner struct{}

func (NoSelfRunner) Handover(context.Context, string, *pipeline.Candidate) (HandoverTicket, error) {
	return HandoverTicket{}, nil
}

func (NoSelfRunner) Finish(context.Context, HandoverTicket) error { return nil }

type FinishStage struct {
	runner SelfRunner
}

func NewFinishStage(runner SelfRunner) *FinishStage {
	return &FinishStage{runner: runner}
}

func (s *FinishStage) Run(ctx context.Context, cycle *pipeline.Cycle) error {
	if cycle.HandoverTicket == nil {
		return nil
	}
	ticket, ok := cycle.HandoverTicket.(HandoverTicket)
	if !ok {
		return nil
	}
	return s.runner.Finish(ctx, ticket)
}
