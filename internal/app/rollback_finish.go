package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type RollbackFinisher struct {
	engines map[string]*engine.Docker
	labels  labelValuer
	logger  *slog.Logger
}

type labelValuer interface {
	Value(container domain.Container, name string) (string, bool)
}

type trackedUpdate struct {
	originalID    string
	originalName  string
	backupName    string
	replacementID string
}

func (x *RollbackFinisher) FinishRollback(ctx context.Context, plan txn.Plan, snapshot string, snap *backup.Snapshotter, candidates []*pipeline.Candidate, tracked []trackedUpdate, reason, detail string) ([]domain.Result, error) {
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
