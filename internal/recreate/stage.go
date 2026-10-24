package recreate

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/labels"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type Stage struct {
	engines   map[string]Engine
	recreate  *Recreator
	snapshots txn.Snapshotter
	verify    txn.Verifier
	ids       *txn.IDGenerator
	labels    *labels.Reader
	store     *state.Store
	stop      time.Duration
	cleanup   bool
	removeVol bool
	logger    *slog.Logger
	now       func() time.Time
}

func NewStage(engines map[string]Engine, recreate *Recreator, snapshots txn.Snapshotter, verify txn.Verifier, ids *txn.IDGenerator, labels *labels.Reader, store *state.Store, stop time.Duration, cleanup, removeVol bool, logger *slog.Logger, now func() time.Time) *Stage {
	return &Stage{
		engines: engines, recreate: recreate, snapshots: snapshots, verify: verify, ids: ids, labels: labels,
		store: store, stop: stop, cleanup: cleanup, removeVol: removeVol, logger: logger, now: now,
	}
}

func (s *Stage) Run(ctx context.Context, cycle *pipeline.Cycle) error {
	if !cycle.Armed {
		return nil
	}
	for _, host := range cycle.ReachableHosts() {
		engine := s.engines[host.Host.Name]
		if engine == nil {
			continue
		}
		groups := groupCandidates(host.Candidates)
		for stack, candidates := range groups {
			if err := s.runStack(ctx, cycle, host, engine, stack, candidates); err != nil {
				return err
			}
		}
	}
	return nil
}

func groupCandidates(candidates []*pipeline.Candidate) map[string][]*pipeline.Candidate {
	groups := map[string][]*pipeline.Candidate{}
	for _, candidate := range candidates {
		if candidate.Result.State != domain.StateWouldUpdate {
			continue
		}
		stack := candidate.Container.Stack()
		groups[stack] = append(groups[stack], candidate)
	}
	return groups
}

func (s *Stage) runStack(ctx context.Context, cycle *pipeline.Cycle, host *pipeline.HostRun, engine Engine, stack string, candidates []*pipeline.Candidate) error {
	if len(candidates) == 0 {
		return nil
	}
	id, err := s.ids.New("txn")
	if err != nil {
		return err
	}
	plan := txn.Plan{ID: id, Host: host.Host.Name, Stacks: []string{stack}}
	for _, candidate := range candidates {
		plan.Targets = append(plan.Targets, txn.Target{
			Container: candidate.Container.Name,
			Image:     candidate.Result.Image,
			Digest:    candidate.Result.CandidateDigest,
		})
	}
	s.logPhase(cycle.ID, id, "snapshot", "")
	if err := s.snapshot(ctx, engine, plan); err != nil {
		return s.failCandidates(candidates, err)
	}
	s.logPhase(cycle.ID, id, "update", "")
	renamed := map[string]string{}
	if err := s.update(ctx, engine, host, plan, candidates, renamed); err != nil {
		return s.rollback(ctx, engine, plan, candidates, renamed, err)
	}
	s.logPhase(cycle.ID, id, "verify", id)
	if err := s.verifyPhase(ctx, plan, candidates); err != nil {
		return s.rollback(ctx, engine, plan, candidates, renamed, err)
	}
	s.logPhase(cycle.ID, id, "commit", id)
	if err := s.commit(ctx, engine, plan, candidates, renamed); err != nil {
		return err
	}
	return nil
}

func (s *Stage) snapshot(ctx context.Context, engine Engine, plan txn.Plan) error {
	if _, err := s.snapshots.Estimate(ctx, plan); err != nil {
		return err
	}
	for _, target := range plan.Targets {
		if err := PullTarget(ctx, engine, target); err != nil {
			return err
		}
		original, err := engine.Inspect(ctx, target.Container)
		if err != nil {
			return err
		}
		if field, ok := checkReproducible(ctx, engine, original); !ok {
			return Failure{State: "failed", Reason: "config_unreproducible", Detail: field}
		}
	}
	snapshot, err := s.snapshots.Take(ctx, plan)
	if err != nil {
		return err
	}
	plan.Snapshot = snapshot
	return s.snapshots.Validate(ctx, snapshot)
}

func (s *Stage) update(ctx context.Context, engine Engine, host *pipeline.HostRun, plan txn.Plan, candidates []*pipeline.Candidate, renamed map[string]string) error {
	settings := Settings{RemoveVolumes: s.removeVol, CommittedAt: s.now()}
	for _, candidate := range candidates {
		original, err := engine.Inspect(ctx, candidate.Container.Name)
		if err != nil {
			return err
		}
		signal := stopSignal(candidate.Container, s.labels, original)
		timeout := stopTimeout(original, s.stop)
		if err := engine.Stop(ctx, original.ID, signal, timeout); err != nil {
			return err
		}
		newName := renamedName(candidate.Container.Name, plan.ID)
		if err := engine.Rename(ctx, original.ID, newName); err != nil {
			return err
		}
		renamed[candidate.Container.Name] = original.ID
		replacement, err := s.recreate.replaceSpec(ctx, plan, original, settings)
		if err != nil {
			return err
		}
		if err := engine.Start(ctx, replacement); err != nil {
			return err
		}
		candidate.Result.ContainerID = replacement
	}
	return nil
}

func (s *Stage) verifyPhase(ctx context.Context, plan txn.Plan, candidates []*pipeline.Candidate) error {
	for _, candidate := range candidates {
		verdict, err := s.verify.Verify(ctx, plan, candidate.Container.Name)
		if err != nil {
			return err
		}
		if !verdict.Passed {
			return Failure{State: "rolled_back", Reason: string(verdict.Reason), Detail: verdict.Check}
		}
	}
	return nil
}

func (s *Stage) commit(ctx context.Context, engine Engine, plan txn.Plan, candidates []*pipeline.Candidate, renamed map[string]string) error {
	for name, id := range renamed {
		if err := engine.Remove(ctx, id, s.removeVol, true); err != nil {
			return err
		}
		if s.cleanup {
			candidate := findCandidate(candidates, name)
			if candidate != nil {
				_ = engine.RemoveImage(ctx, candidate.Result.ImageID)
			}
		}
	}
	for _, candidate := range candidates {
		candidate.Result = candidate.Result.Conclude(domain.StateUpdated, "", "")
		candidate.Result.Transaction = plan.ID
		candidate.Result.Snapshot = plan.Snapshot
	}
	return s.store.Update(func(document *state.Document) error {
		document.Transactions = append(document.Transactions, state.TransactionRecord{
			ID: plan.ID, Host: plan.Host, Stacks: plan.Stacks, State: string(txn.StateCommitted),
			Snapshot: plan.Snapshot, Result: string(domain.StateUpdated), Time: s.now(),
		})
		return nil
	})
}

func (s *Stage) rollback(ctx context.Context, engine Engine, plan txn.Plan, candidates []*pipeline.Candidate, renamed map[string]string, cause error) error {
	for _, candidate := range candidates {
		if candidate.Result.ContainerID != "" {
			_ = engine.Remove(ctx, candidate.Result.ContainerID, false, true)
		}
	}
	for name, id := range renamed {
		_ = engine.Rename(ctx, id, name)
		_ = engine.Start(ctx, id)
	}
	reason, state := failureOutcome(cause)
	for _, candidate := range candidates {
		candidate.Result = candidate.Result.Conclude(domain.State(state), reason, detailOf(cause))
		candidate.Result.Transaction = plan.ID
	}
	return nil
}

func (s *Stage) failCandidates(candidates []*pipeline.Candidate, cause error) error {
	reason, state := failureOutcome(cause)
	for _, candidate := range candidates {
		candidate.Result = candidate.Result.Conclude(domain.State(state), reason, detailOf(cause))
	}
	return nil
}

func failureOutcome(cause error) (string, string) {
	var failure Failure
	if errors.As(cause, &failure) {
		return failure.Reason, failure.State
	}
	return "engine_error", string(domain.StateFailed)
}

func detailOf(cause error) string {
	var failure Failure
	if errors.As(cause, &failure) {
		return failure.Detail
	}
	if cause == nil {
		return ""
	}
	return cause.Error()
}

func findCandidate(candidates []*pipeline.Candidate, name string) *pipeline.Candidate {
	for _, candidate := range candidates {
		if candidate.Container.Name == name {
			return candidate
		}
	}
	return nil
}

func (s *Stage) logPhase(cycleID, txnID, phase, txn string) {
	value := txn
	if value == "" {
		value = txnID
	}
	if phase == "snapshot" && txn == "" {
		value = ""
	}
	attrs := []any{"cycle", cycleID, "phase", phase}
	if value != "" {
		attrs = append(attrs, "txn", value)
	}
	s.logger.Info("transaction phase", attrs...)
}
