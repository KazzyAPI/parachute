package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type ExecuteStage struct {
	armed        bool
	journal      *state.Journal
	snapshotters map[string]*backup.Snapshotter
	retention    int
	executor     *stackExecutor
	logger       *slog.Logger
}

func NewExecuteStage(armed bool, journal *state.Journal, snapshotters map[string]*backup.Snapshotter, retention int, executor *stackExecutor, logger *slog.Logger) *ExecuteStage {
	return &ExecuteStage{armed: armed, journal: journal, snapshotters: snapshotters, retention: retention, executor: executor, logger: logger}
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
		groups[candidate.Result.Stack] = append(groups[candidate.Result.Stack], candidate)
	}
	return groups
}

func (s *ExecuteStage) runStack(ctx context.Context, cycle *pipeline.Cycle, host *pipeline.HostRun, stack string, candidates []*pipeline.Candidate) error {
	if len(candidates) == 0 {
		return nil
	}
	snap := s.snapshotters[host.Host.Name]
	if snap == nil {
		return fmt.Errorf("no snapshotter for host %s", host.Host.Name)
	}
	txnID, err := s.executor.ids.New("txn")
	if err != nil {
		return err
	}
	s.logger.Info("phase snapshot", "cycle", cycle.ID, "phase", "snapshot", "txn", txnID)
	plan := txn.Plan{ID: txnID, Host: host.Host.Name, Stacks: []string{stack}}
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
	_ = s.journal.Advance(ctx, txnID, txn.StateUpdate)
	for _, candidate := range candidates {
		candidate.Result.Snapshot = snapshotID
		candidate.Result.Transaction = txnID
	}
	outcome, err := s.executor.run(ctx, plan, snapshotID, snap, candidates)
	if err != nil {
		return err
	}
	for index, candidate := range candidates {
		candidate.Result = outcome[index]
	}
	_ = s.journal.Finish(ctx, txnID, txnOutcome(outcome[0]))
	if outcome[0].State == domain.StateUpdated || outcome[0].State == domain.StateRolledBack {
		_ = snap.Retain(ctx, host.Host.Name, stack, s.retention)
	}
	return nil
}

func txnOutcome(result domain.Result) txn.Outcome {
	state := txn.StateCommitted
	if result.State == domain.StateRolledBack {
		state = txn.StateRolledBack
	}
	if result.State == domain.StateRollbackFailed {
		state = txn.StateRollbackFail
	}
	if result.State == domain.StateFailed {
		state = txn.StateAborted
	}
	return txn.Outcome{State: state, Result: result.State, Reason: txn.Reason(result.Reason)}
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
	engines      map[string]*engine.Docker
	snapshotters map[string]*backup.Snapshotter
	ids          *txn.IDGenerator
	labels       labelValuer
	cfg          updateSettings
	logger       *slog.Logger
	clock        Clock
}

type updateSettings struct {
	stopTimeout   time.Duration
	verifyTimeout time.Duration
	verifyStable  time.Duration
	helperImage   string
	backupDefault string
}

type labelValuer interface {
	Value(container domain.Container, name string) (string, bool)
}

type trackedUpdate struct {
	candidate     *pipeline.Candidate
	originalID    string
	originalName  string
	backupName    string
	replacementID string
	started       bool
}

func (x *stackExecutor) run(ctx context.Context, plan txn.Plan, snapshot string, snap *backup.Snapshotter, candidates []*pipeline.Candidate) ([]domain.Result, error) {
	docker := x.engines[plan.Host]
	if docker == nil {
		return nil, fmt.Errorf("no engine for host %s", plan.Host)
	}
	x.logger.Info("phase update", "txn", plan.ID, "phase", "update")
	for _, candidate := range candidates {
		if err := docker.Pull(ctx, candidate.Result.Image, candidate.Result.CandidateDigest, ""); err != nil {
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, nil, "pull_failed", err.Error())
		}
		if err := docker.Tag(ctx, candidate.Result.Image+"@"+candidate.Result.CandidateDigest, candidate.Result.Image); err != nil {
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, nil, "pull_failed", err.Error())
		}
	}
	var tracked []trackedUpdate
	for _, candidate := range candidates {
		suffix := plan.ID[len(plan.ID)-6:]
		backupName := candidate.Container.Name + "-parachute-" + suffix
		if err := docker.Stop(ctx, candidate.Container.ID, "SIGTERM", x.cfg.stopTimeout); err != nil {
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, tracked, "engine_error", err.Error())
		}
		if err := docker.Rename(ctx, candidate.Container.ID, backupName); err != nil {
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, tracked, "engine_error", err.Error())
		}
		spec, err := docker.Inspect(ctx, candidate.Container.ID)
		if err != nil {
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, tracked, "config_unreproducible", err.Error())
		}
		spec.Name = candidate.Container.Name
		if spec.Config != nil {
			spec.Config.Image = candidate.Result.Image
			if spec.Config.Labels == nil {
				spec.Config.Labels = map[string]string{}
			}
			spec.Config.Labels["parachute.transaction"] = plan.ID
			snapshotLabel := snapshot
			if mode, ok := x.labels.Value(candidate.Container, "backup"); ok {
				if mode == "none" {
					snapshotLabel = "none"
				}
			} else if x.cfg.backupDefault == "none" {
				snapshotLabel = "none"
			}
			spec.Config.Labels["parachute.snapshot"] = snapshotLabel
			spec.Config.Labels["parachute.updated-at"] = x.clock.Now().UTC().Format(time.RFC3339)
			spec.Config.Labels["parachute.previous-image"] = candidate.Result.Image + "@" + candidate.Result.CandidateDigest
		}
		replacement, err := docker.Create(ctx, spec)
		if err != nil {
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, tracked, "config_unreproducible", err.Error())
		}
		tracked = append(tracked, trackedUpdate{
			candidate: candidate, originalID: candidate.Container.ID, originalName: candidate.Container.Name,
			backupName: backupName, replacementID: replacement,
		})
		candidate.Result.ContainerID = replacement
		if err := docker.Start(ctx, replacement); err != nil {
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, tracked, "verify_failed", err.Error())
		}
		for index := range tracked {
			tracked[index].started = true
		}
	}
	x.logger.Info("phase verify", "txn", plan.ID, "phase", "verify")
	for _, item := range tracked {
		if err := x.verify(ctx, docker, item.candidate); err != nil {
			reason := "verify_failed"
			if err.Error() == "verify_timeout" {
				reason = "verify_timeout"
			}
			return x.finishRollback(ctx, plan, snapshot, snap, candidates, tracked, reason, err.Error())
		}
	}
	x.logger.Info("phase commit", "txn", plan.ID, "phase", "commit")
	for _, item := range tracked {
		_ = docker.Remove(ctx, item.originalID, false, true)
		item.candidate.Result = item.candidate.Result.Conclude(domain.StateUpdated, "", "")
		item.candidate.Result.Transaction = plan.ID
		item.candidate.Result.Snapshot = snapshot
	}
	return resultsFrom(candidates), nil
}

func (x *stackExecutor) verify(ctx context.Context, docker *engine.Docker, candidate *pipeline.Candidate) error {
	stable := x.cfg.verifyStable
	if stable <= 0 {
		stable = time.Second
	}
	deadline := x.clock.Now().Add(x.cfg.verifyTimeout)
	started := x.clock.Now()
	for x.clock.Now().Before(deadline) {
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
		if time.Since(started) >= stable {
			if cmd, ok := x.labels.Value(candidate.Container, "verify.cmd"); ok {
				result, err := docker.Exec(ctx, candidate.Result.ContainerID, []string{"sh", "-c", cmd}, nil, x.cfg.verifyTimeout, 1<<20)
				if err != nil || result.ExitCode != 0 {
					return errors.New("verify_failed")
				}
			}
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return errors.New("verify_timeout")
}

func (x *stackExecutor) finishRollback(ctx context.Context, plan txn.Plan, snapshot string, snap *backup.Snapshotter, candidates []*pipeline.Candidate, tracked []trackedUpdate, reason, detail string) ([]domain.Result, error) {
	x.logger.Info("phase rollback", "txn", plan.ID, "phase", "rollback")
	docker := x.engines[plan.Host]
	for _, item := range tracked {
		if item.replacementID != "" {
			_ = docker.Remove(ctx, item.replacementID, false, true)
		}
		if item.backupName != "" {
			_ = docker.Rename(ctx, item.originalID, item.originalName)
			_ = docker.Start(ctx, item.originalID)
		}
	}
	for _, candidate := range candidates {
		started := false
		for _, item := range tracked {
			if item.originalID == candidate.Container.ID {
				started = true
				break
			}
		}
		if !started {
			_ = docker.Start(ctx, candidate.Container.ID)
		}
	}
	invalidSnapshot := false
	dataRestored := false
	if snapshot != "" && snapshot != "none" {
		if err := snap.ValidateTransaction(ctx, snapshot, plan.Host, plan.Stacks[0], plan.ID); err != nil {
			invalidSnapshot = true
		} else if err := snap.Restore(ctx, snapshot); err != nil {
			if strings.Contains(err.Error(), "snapshot_invalid") {
				invalidSnapshot = true
			}
		} else {
			dataRestored = true
		}
	}
	skipped, _ := snap.SkippedFromSnapshot(ctx, snapshot)
	for _, candidate := range candidates {
		details := detail
		mode, _ := x.labels.Value(candidate.Container, "backup")
		switch {
		case mode == "none":
			details = joinDetail(details, "data not restored: backup=none")
		case invalidSnapshot:
			details = joinDetail(details, "data not restored: snapshot invalid")
		case dataRestored:
			details = joinDetail(details, "data restored: yes")
		}
		for _, item := range skipped {
			if item.Container == candidate.Container.Name {
				details = joinDetail(details, "data not restored: system mount "+item.Destination)
			}
		}
		state := domain.StateRolledBack
		rollbackReason := reason
		if invalidSnapshot {
			state = domain.StateRollbackFailed
			rollbackReason = "snapshot_invalid"
		}
		candidate.Result = candidate.Result.Conclude(state, rollbackReason, details)
		candidate.Result.Transaction = plan.ID
		candidate.Result.Snapshot = snapshot
	}
	return resultsFrom(candidates), nil
}

func joinDetail(left, right string) string {
	if left == "" {
		return right
	}
	return left + "; " + right
}

func resultsFrom(candidates []*pipeline.Candidate) []domain.Result {
	out := make([]domain.Result, len(candidates))
	for index, candidate := range candidates {
		out[index] = candidate.Result
	}
	return out
}
