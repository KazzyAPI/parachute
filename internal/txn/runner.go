package txn

import (
	"context"
	"fmt"
)

type Runner struct {
	journal   Journal
	snapshots Snapshotter
	recreate  Recreator
	verify    Verifier
	hooks     Hooks
	ceiling   *Ceiling
}

func NewRunner(journal Journal, snapshots Snapshotter, recreate Recreator, verify Verifier, hooks Hooks, ceiling *Ceiling) *Runner {
	if hooks == nil {
		hooks = NoOpHooks{}
	}
	return &Runner{
		journal: journal, snapshots: snapshots, recreate: recreate, verify: verify,
		hooks: hooks, ceiling: ceiling,
	}
}

func (r *Runner) Run(ctx context.Context, plan Plan) error {
	if r.journal == nil || r.snapshots == nil || r.recreate == nil || r.verify == nil {
		return fmt.Errorf("transaction runner is not wired")
	}
	record := OpenRecord{ID: plan.ID, Host: plan.Host, Stacks: plan.Stacks, State: StateSnapshot, Snapshot: plan.Snapshot, SelfUpdate: plan.SelfUpdate}
	if err := r.journal.Begin(ctx, record); err != nil {
		return err
	}
	if r.ceiling != nil {
		r.ceiling.Begin()
	}
	if err := r.snapshotPhase(ctx, plan); err != nil {
		return r.end(ctx, plan.ID, StateAborted, "failed", err)
	}
	if plan.SelfUpdate {
		return r.end(ctx, plan.ID, StateHandover, "", fmt.Errorf("handover is not implemented"))
	}
	if err := r.advance(ctx, plan.ID, StateUpdate); err != nil {
		return err
	}
	if err := r.updatePhase(ctx, plan); err != nil {
		return r.rollback(ctx, plan, err)
	}
	if err := r.advance(ctx, plan.ID, StateVerify); err != nil {
		return err
	}
	if err := r.verifyPhase(ctx, plan); err != nil {
		return r.rollback(ctx, plan, err)
	}
	return r.end(ctx, plan.ID, StateCommitted, "updated", nil)
}

func (r *Runner) snapshotPhase(ctx context.Context, plan Plan) error {
	if _, err := r.snapshots.Estimate(ctx, plan); err != nil {
		return err
	}
	if err := r.hooks.PreUpdate(ctx, plan); err != nil {
		return err
	}
	snapshot, err := r.snapshots.Take(ctx, plan)
	if err != nil {
		return err
	}
	plan.Snapshot = snapshot
	if err := r.snapshots.Validate(ctx, snapshot); err != nil {
		return err
	}
	return nil
}

func (r *Runner) updatePhase(ctx context.Context, plan Plan) error {
	for _, container := range plan.Containers {
		if _, err := r.recreate.Replacement(ctx, plan, container); err != nil {
			return err
		}
	}
	return r.hooks.PostUpdate(ctx, plan)
}

func (r *Runner) verifyPhase(ctx context.Context, plan Plan) error {
	if err := r.hooks.PreCheck(ctx, plan); err != nil {
		return err
	}
	for _, container := range plan.Containers {
		verdict, err := r.verify.Verify(ctx, plan, container)
		if err != nil {
			return err
		}
		if !verdict.Passed {
			return fmt.Errorf("%s", verdict.Reason)
		}
	}
	return r.hooks.PostCheck(ctx, plan)
}

func (r *Runner) rollback(ctx context.Context, plan Plan, cause error) error {
	if err := r.advance(ctx, plan.ID, StateRollingBack); err != nil {
		return err
	}
	reason := "rollback"
	if cause != nil {
		reason = cause.Error()
	}
	return r.end(ctx, plan.ID, StateRolledBack, reason, cause)
}

func (r *Runner) advance(ctx context.Context, id string, state State) error {
	if err := r.journal.Advance(ctx, id, state); err != nil {
		return err
	}
	if r.ceiling != nil && state != StateRollingBack && r.ceiling.Expired() {
		return fmt.Errorf("transaction_timeout")
	}
	return nil
}

func (r *Runner) SelfUpdateLast() bool { return true }

func (r *Runner) end(ctx context.Context, id string, state State, result string, cause error) error {
	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	if err := r.journal.Finish(ctx, id, state, result, reason); err != nil {
		return err
	}
	return cause
}
