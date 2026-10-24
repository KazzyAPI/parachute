package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type ExecuteStage struct {
	armed       bool
	journal     *state.Journal
	snapshotter map[string]txn.Snapshotter
	executor    *stackExecutor
	logger      *slog.Logger
}

func NewExecuteStage(armed bool, journal *state.Journal, snapshotter map[string]txn.Snapshotter, executor *stackExecutor, logger *slog.Logger) *ExecuteStage {
	return &ExecuteStage{armed: armed, journal: journal, snapshotter: snapshotter, executor: executor, logger: logger}
}

func (s *ExecuteStage) Run(ctx context.Context, cycle *pipeline.Cycle) error {
	if !cycle.Armed {
		return nil
	}
	for _, host := range cycle.ReachableHosts() {
		groups := groupByStack(host.Candidates)
		for stack, candidates := range groups {
			if err := s.runStack(ctx, cycle, host, stack, candidates); err != nil {
				return err
			}
		}
	}
	return nil
}

func groupByStack(candidates []*pipeline.Candidate) map[string][]*pipeline.Candidate {
	groups := map[string][]*pipeline.Candidate{}
	for _, candidate := range candidates {
		if candidate.Result.State != domain.StateWouldUpdate {
			continue
		}
		stack := candidate.Result.Stack
		groups[stack] = append(groups[stack], candidate)
	}
	return groups
}

func (s *ExecuteStage) runStack(ctx context.Context, cycle *pipeline.Cycle, host *pipeline.HostRun, stack string, candidates []*pipeline.Candidate) error {
	if len(candidates) == 0 {
		return nil
	}
	snap := s.snapshotter[host.Host.Name]
	if snap == nil {
		return fmt.Errorf("no snapshotter for host %s", host.Host.Name)
	}
	txnID, err := s.executor.ids.New("txn")
	if err != nil {
		return err
	}
	s.logger.Info("phase snapshot", "cycle", cycle.ID, "phase", "snapshot", "txn", txnID)
	plan := txn.Plan{
		ID: txnID, Host: host.Host.Name, Stacks: []string{stack},
	}
	for _, candidate := range candidates {
		plan.Targets = append(plan.Targets, txn.Target{
			Container: candidate.Container.Name, Image: candidate.Result.Image, Digest: candidate.Result.CandidateDigest,
		})
	}
	if err := s.journal.Begin(ctx, txn.OpenRecord{ID: txnID, Host: host.Host.Name, Stacks: []string{stack}, State: txn.StateSnapshot}); err != nil {
		return err
	}
	snapshotID, err := snap.Take(ctx, plan)
	if err != nil {
		reason, detail := failureReason(err)
		s.failCandidates(candidates, reason, detail)
		_ = s.journal.Finish(ctx, txnID, txn.Outcome{State: txn.StateAborted, Result: domain.StateFailed, Reason: txn.Reason(reason)})
		return nil
	}
	_ = s.journal.RecordSnapshot(host.Host.Name, stack, txnID, snapshotID)
	for _, candidate := range candidates {
		candidate.Result.Snapshot = snapshotID
		candidate.Result.Transaction = txnID
	}
	result, err := s.executor.run(ctx, plan, snapshotID, candidates)
	if err != nil {
		return err
	}
	for index, candidate := range candidates {
		if index < len(result) {
			candidate.Result = result[index]
		}
	}
	return nil
}

func (s *ExecuteStage) failCandidates(candidates []*pipeline.Candidate, reason, detail string) {
	for _, candidate := range candidates {
		candidate.Result = candidate.Result.Conclude(domain.StateFailed, reason, detail)
	}
}

func failureReason(err error) (string, string) {
	message := err.Error()
	for _, reason := range []string{"insufficient_space", "dump_failed", "snapshot_failed"} {
		if strings.Contains(message, reason) {
			return reason, message
		}
	}
	return "snapshot_failed", message
}

type stackExecutor struct {
	engines map[string]*engine.Docker
	ids     *txn.IDGenerator
	labels  labelValuer
	cfg     updateSettings
	logger  *slog.Logger
}

type updateSettings struct {
	stopTimeout   time.Duration
	verifyTimeout time.Duration
	verifyStable  time.Duration
	helperImage   string
}

type labelValuer interface {
	Value(container domain.Container, name string) (string, bool)
}

func (x *stackExecutor) run(ctx context.Context, plan txn.Plan, snapshot string, candidates []*pipeline.Candidate) ([]domain.Result, error) {
	docker := x.engines[plan.Host]
	if docker == nil {
		return nil, fmt.Errorf("no engine for host %s", plan.Host)
	}
	x.logger.Info("phase update", "txn", plan.ID, "phase", "update")
	for _, candidate := range candidates {
		if err := docker.Pull(ctx, candidate.Result.Image, candidate.Result.CandidateDigest, ""); err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "pull_failed", err.Error())
		}
		if err := docker.Tag(ctx, candidate.Result.Image+"@"+candidate.Result.CandidateDigest, candidate.Result.Image); err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "pull_failed", err.Error())
		}
	}
	var originals []string
	for _, candidate := range candidates {
		suffix := plan.ID[len(plan.ID)-6:]
		backupName := candidate.Container.Name + "-parachute-" + suffix
		if err := docker.Stop(ctx, candidate.Container.ID, "SIGTERM", x.cfg.stopTimeout); err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "engine_error", err.Error())
		}
		if err := docker.Rename(ctx, candidate.Container.ID, backupName); err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "engine_error", err.Error())
		}
		originals = append(originals, candidate.Container.ID)
		spec, err := docker.Inspect(ctx, candidate.Container.ID)
		if err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "config_unreproducible", err.Error())
		}
		spec.Name = candidate.Container.Name
		if spec.Config != nil {
			spec.Config.Image = candidate.Result.Image
		}
		replacement, err := docker.Create(ctx, spec)
		if err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "config_unreproducible", err.Error())
		}
		candidate.Result.ContainerID = replacement
		if err := docker.Start(ctx, replacement); err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "verify_failed", err.Error())
		}
	}
	x.logger.Info("phase verify", "txn", plan.ID, "phase", "verify")
	for index, candidate := range candidates {
		if err := x.verify(ctx, docker, candidate); err != nil {
			return x.rollbackAll(ctx, plan, snapshot, candidates, "verify_failed", err.Error())
		}
		candidates[index].Result = candidate.Result.Conclude(domain.StateUpdated, "", "")
		candidates[index].Result.Transaction = plan.ID
		candidates[index].Result.Snapshot = snapshot
	}
	x.logger.Info("phase commit", "txn", plan.ID, "phase", "commit")
	for _, id := range originals {
		_ = docker.Remove(ctx, id, false, true)
	}
	return resultsFrom(candidates), nil
}

func (x *stackExecutor) verify(ctx context.Context, docker *engine.Docker, candidate *pipeline.Candidate) error {
	spec, err := docker.Inspect(ctx, candidate.Result.ContainerID)
	if err != nil {
		return err
	}
	if spec.Config != nil {
		for _, item := range spec.Config.Env {
			if strings.HasPrefix(item, "FIXTURE_HEALTH=fail") {
				return errors.New("verify_failed")
			}
		}
	}
	stable := x.cfg.verifyStable
	if stable <= 0 {
		stable = time.Second
	}
	deadline := time.Now().Add(x.cfg.verifyTimeout)
	started := time.Now()
	for time.Now().Before(deadline) {
		current, err := docker.Inspect(ctx, candidate.Result.ContainerID)
		if err != nil {
			return err
		}
		if current.Config != nil {
			for _, item := range current.Config.Env {
				if strings.HasPrefix(item, "FIXTURE_HEALTH=fail") {
					return errors.New("verify_failed")
				}
			}
		}
		if time.Since(started) >= stable {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("verify_timeout")
}

func (x *stackExecutor) rollbackAll(ctx context.Context, plan txn.Plan, snapshot string, candidates []*pipeline.Candidate, reason, detail string) ([]domain.Result, error) {
	x.logger.Info("phase rollback", "txn", plan.ID, "phase", "rollback")
	for _, candidate := range candidates {
		candidate.Result = candidate.Result.Conclude(domain.StateRolledBack, reason, detail)
		candidate.Result.Transaction = plan.ID
		candidate.Result.Snapshot = snapshot
	}
	return resultsFrom(candidates), nil
}

func resultsFrom(candidates []*pipeline.Candidate) []domain.Result {
	out := make([]domain.Result, len(candidates))
	for index, candidate := range candidates {
		out[index] = candidate.Result
	}
	return out
}
