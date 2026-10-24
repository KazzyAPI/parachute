package pipeline

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/hosts"
)

type Sink interface {
	Publish(ctx context.Context, report domain.Report) error
}

type Clock interface {
	Now() time.Time
}

type ReportStage struct {
	clock  Clock
	sinks  []Sink
	logger *slog.Logger
}

func NewReportStage(clock Clock, logger *slog.Logger, sinks ...Sink) *ReportStage {
	return &ReportStage{clock: clock, sinks: sinks, logger: logger}
}

func (s *ReportStage) Run(ctx context.Context, cycle *Cycle) error {
	report := domain.Report{CycleID: cycle.ID, Started: cycle.Started, Ended: s.clock.Now(), Armed: cycle.Armed}
	for _, host := range cycle.Hosts {
		hostReport := domain.HostReport{Name: host.Host.Name, Endpoint: host.Host.Endpoint, Reachable: host.Reachable}
		for _, candidate := range host.Candidates {
			hostReport.Results = append(hostReport.Results, candidate.Result)
			s.log(ctx, cycle.ID, candidate)
		}
		slices.SortStableFunc(hostReport.Results, func(left, right domain.Result) int {
			return cmp.Or(cmp.Compare(left.Stack, right.Stack), cmp.Compare(left.Service, right.Service), cmp.Compare(left.Container, right.Container))
		})
		report.Hosts = append(report.Hosts, hostReport)
	}
	if err := hosts.ValidateTransactions(report.Results()); err != nil {
		s.logger.Error("inconsistent transaction IDs in report", "cycle", cycle.ID, "error", err.Error())
	}
	cycle.Report = report
	counts := report.Counts()
	s.logger.Info("cycle finished", "cycle", cycle.ID, "scanned", counts.Scanned, "would_update", counts.WouldUpdate,
		"held", counts.Held, "fresh", counts.Fresh, "skipped", counts.Skipped, "failed", counts.Failed)
	for _, sink := range s.sinks {
		if err := sink.Publish(ctx, report); err != nil {
			s.logger.Error("cannot publish cycle report", "cycle", cycle.ID, "error", err.Error())
		}
	}
	return nil
}

func (s *ReportStage) log(ctx context.Context, cycleID string, candidate *Candidate) {
	result := candidate.Result
	attributes := []any{"cycle", cycleID, "host", result.Host, "stack", result.Stack, "service", result.Service,
		"container", result.Container, "image", result.Image, "state", string(result.State)}
	if result.Reason != "" {
		attributes = append(attributes, "reason", result.Reason)
	}
	if result.CandidateDigest != "" {
		attributes = append(attributes, "candidate_digest", result.CandidateDigest)
	}
	if result.Detail != "" {
		attributes = append(attributes, "detail", result.Detail)
	}
	s.logger.Log(ctx, result.State.Level().Slog(), "container checked", attributes...)
}
