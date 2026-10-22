package pipeline

import (
	"context"
	"log/slog"
	"sort"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
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
	levels map[domain.Level]slog.Level
}

func NewReportStage(clock Clock, logger *slog.Logger, sinks ...Sink) *ReportStage {
	return &ReportStage{clock: clock, sinks: sinks, logger: logger, levels: map[domain.Level]slog.Level{
		domain.LevelDebug: slog.LevelDebug, domain.LevelInfo: slog.LevelInfo,
		domain.LevelWarn: slog.LevelWarn, domain.LevelError: slog.LevelError,
	}}
}

func (s *ReportStage) Run(ctx context.Context, cycle *Cycle) error {
	report := domain.Report{CycleID: cycle.ID, Started: cycle.Started, Ended: s.clock.Now(), Armed: cycle.Armed}
	for _, host := range cycle.Hosts {
		hostReport := domain.HostReport{Name: host.Host.Name, Endpoint: host.Host.Endpoint, Reachable: host.Reachable}
		for _, candidate := range host.Candidates {
			hostReport.Results = append(hostReport.Results, candidate.Result)
			s.log(ctx, cycle.ID, candidate)
		}
		sort.SliceStable(hostReport.Results, func(i, j int) bool {
			left, right := hostReport.Results[i], hostReport.Results[j]
			if left.Stack != right.Stack {
				return left.Stack < right.Stack
			}
			return left.Service+left.Container < right.Service+right.Container
		})
		report.Hosts = append(report.Hosts, hostReport)
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
	if candidate.Finding.Detail != "" {
		attributes = append(attributes, "detail", candidate.Finding.Detail)
	}
	s.logger.Log(ctx, s.levels[result.State.Level()], "container checked", attributes...)
}
