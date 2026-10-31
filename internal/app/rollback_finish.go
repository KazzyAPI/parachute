package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type rollbackEngine interface {
	Containers(ctx context.Context) ([]domain.Container, error)
	Runtime(ctx context.Context, id string) (engine.Runtime, error)
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Remove(ctx context.Context, id string, anonymousVolumes, force bool) error
	Rename(ctx context.Context, id, name string) error
	Start(ctx context.Context, id string) error
}

type rollbackSnapshots interface {
	ValidateTransaction(ctx context.Context, snapshot, host, stack, transaction string) error
	Restore(ctx context.Context, snapshot string) error
	SkippedFromSnapshot(ctx context.Context, snapshot string) ([]backup.SkippedRecord, error)
}

type labelValuer interface {
	Value(container domain.Container, name string) (string, bool)
}

type RollbackFinisher struct {
	engine    rollbackEngine
	snapshots rollbackSnapshots
	verifier  txn.Verifier
	labels    labelValuer
	logger    *slog.Logger
}

func NewRollbackFinisher(engine rollbackEngine, snapshots rollbackSnapshots, verifier txn.Verifier, labels labelValuer, logger *slog.Logger) *RollbackFinisher {
	return &RollbackFinisher{engine: engine, snapshots: snapshots, verifier: verifier, labels: labels, logger: logger}
}

type restoreReport struct {
	dataRestored    bool
	invalidSnapshot bool
	failure         error
}

func (x *RollbackFinisher) FinishRollback(ctx context.Context, plan txn.Plan, originals []txn.Original, candidates []*pipeline.Candidate, final domain.State, reason, detail string) (txn.Outcome, error) {
	var report restoreReport
	started, err := x.removeReplacements(ctx, plan.ID)
	report.fail(err)
	if started && plan.Snapshot != "" && plan.Snapshot != "none" {
		x.restoreData(ctx, plan, &report)
	}
	restored, restarted, err := x.restoreOriginals(ctx, plan, originals)
	report.fail(err)
	if report.failure == nil {
		report.fail(x.verifyOriginals(ctx, plan, restarted))
	}
	skipped, _ := x.snapshots.SkippedFromSnapshot(ctx, plan.Snapshot)
	outcome := txn.Outcome{State: txn.StateRolledBack, Result: final, Reason: txn.Reason(reason)}
	switch {
	case report.invalidSnapshot:
		outcome = txn.Outcome{State: txn.StateRollbackFail, Result: domain.StateRollbackFailed, Reason: "snapshot_invalid"}
	case report.failure != nil:
		x.logger.Error("rollback failed", "txn", plan.ID, "error", report.failure.Error())
		outcome = txn.Outcome{State: txn.StateRollbackFail, Result: domain.StateRollbackFailed, Reason: "restore_failed"}
	}
	for _, candidate := range candidates {
		if id, ok := restored[candidate.Container.Name]; ok {
			candidate.Container.ID = id
		}
		details := detail
		mode, _ := x.labels.Value(candidate.Container, "backup")
		switch {
		case mode == "none":
			details = joinDetail(details, "data not restored: backup=none")
		case report.invalidSnapshot:
			details = joinDetail(details, "data not restored: snapshot invalid")
		case report.dataRestored:
			details = joinDetail(details, "data restored: yes")
		}
		for _, item := range skipped {
			if item.Container == candidate.Container.Name {
				details = joinDetail(details, "data not restored: system mount "+item.Destination)
			}
		}
		if report.failure != nil && !report.invalidSnapshot {
			details = joinDetail(details, report.failure.Error())
		}
		candidate.Result = candidate.Result.Conclude(outcome.Result, string(outcome.Reason), details)
		candidate.Result.Transaction = plan.ID
		candidate.Result.Snapshot = plan.Snapshot
	}
	return outcome, nil
}

func (r *restoreReport) fail(err error) {
	if err != nil && r.failure == nil {
		r.failure = err
	}
}

func (x *RollbackFinisher) removeReplacements(ctx context.Context, transaction string) (bool, error) {
	containers, err := x.engine.Containers(ctx)
	if err != nil {
		return false, err
	}
	started := false
	var failure error
	for _, container := range containers {
		if container.Labels["parachute.transaction"] != transaction || container.Labels["parachute.helper"] == "true" {
			continue
		}
		if runtime, err := x.engine.Runtime(ctx, container.ID); err != nil || runtime.Status != "created" {
			started = true
		}
		if err := x.engine.Remove(ctx, container.ID, false, true); err != nil && failure == nil {
			failure = err
		}
	}
	return started, failure
}

func (x *RollbackFinisher) restoreData(ctx context.Context, plan txn.Plan, report *restoreReport) {
	if err := x.snapshots.ValidateTransaction(ctx, plan.Snapshot, plan.Host, plan.Stacks[0], plan.ID); err != nil {
		report.invalidSnapshot = true
		return
	}
	if err := x.snapshots.Restore(ctx, plan.Snapshot); err != nil {
		if strings.Contains(err.Error(), "snapshot_invalid") {
			report.invalidSnapshot = true
			return
		}
		report.fail(err)
		return
	}
	report.dataRestored = true
}

func (x *RollbackFinisher) restoreOriginals(ctx context.Context, plan txn.Plan, originals []txn.Original) (map[string]string, []txn.Original, error) {
	restored := map[string]string{}
	var restarted []txn.Original
	suffix := "-parachute-" + plan.ID[len(plan.ID)-6:]
	var failure error
	for _, original := range originals {
		spec, err := x.engine.Inspect(ctx, original.ID)
		if err != nil {
			failure = firstError(failure, err)
			continue
		}
		name := spec.Name
		if base, ok := strings.CutSuffix(name, suffix); ok {
			if err := x.engine.Rename(ctx, original.ID, base); err != nil {
				failure = firstError(failure, err)
				continue
			}
			name = base
		}
		restored[name] = original.ID
	}
	for _, original := range originals {
		if !original.Running {
			continue
		}
		if runtime, err := x.engine.Runtime(ctx, original.ID); err == nil && runtime.Running {
			continue
		}
		if err := x.engine.Start(ctx, original.ID); err != nil {
			failure = firstError(failure, err)
			continue
		}
		restarted = append(restarted, original)
	}
	return restored, restarted, failure
}

func (x *RollbackFinisher) verifyOriginals(ctx context.Context, plan txn.Plan, originals []txn.Original) error {
	for _, original := range originals {
		if !original.Running {
			continue
		}
		verdict, err := x.verifier.VerifyBase(ctx, plan, original.ID)
		if err != nil {
			return err
		}
		if !verdict.Passed {
			return fmt.Errorf("original %s failed %s: %s", original.ID, verdict.Check, verdict.Output)
		}
	}
	return nil
}

func firstError(current, next error) error {
	if current != nil {
		return current
	}
	return next
}

func joinDetail(left, right string) string {
	if left == "" {
		return right
	}
	return left + "; " + right
}
