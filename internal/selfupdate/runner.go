package selfupdate

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/handover"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/registry"
	"github.com/KazzyAPI/parachute/internal/txn"
	"github.com/KazzyAPI/parachute/internal/verify"
)

type Engine interface {
	graph.Engine
	Containers(ctx context.Context) ([]domain.Container, error)
	Runtime(ctx context.Context, id string) (engine.Runtime, error)
}

type Runner struct {
	engine    Engine
	journal   txn.Journal
	snapshot  txn.Snapshotter
	recreator txn.Recreator
	verifier  txn.Verifier
	files     handover.FileSystem
	stateDir  string
	ids       *txn.IDGenerator
	clock     graph.Clock
	logger    *slog.Logger
	options   graph.Options
	auth      func(registry.Reference) engine.RegistryAuth
}

func NewRunner(
	engine Engine,
	journal txn.Journal,
	snapshot txn.Snapshotter,
	recreator txn.Recreator,
	verifier txn.Verifier,
	files handover.FileSystem,
	stateDir string,
	ids *txn.IDGenerator,
	clock graph.Clock,
	logger *slog.Logger,
	options graph.Options,
	auth func(registry.Reference) engine.RegistryAuth,
) *Runner {
	return &Runner{
		engine: engine, journal: journal, snapshot: snapshot, recreator: recreator, verifier: verifier,
		files: files, stateDir: stateDir, ids: ids, clock: clock, logger: logger, options: options, auth: auth,
	}
}

func (r *Runner) Handover(ctx context.Context, host string, own *pipeline.Candidate) (graph.HandoverTicket, error) {
	if own == nil || own.Result.State != domain.StateWouldUpdate {
		return graph.HandoverTicket{}, nil
	}
	id, err := r.ids.New("txn")
	if err != nil {
		return graph.HandoverTicket{}, err
	}
	originalID := own.Container.ID
	originalName := own.Container.Name
	target := txn.Target{
		Container: own.Container.Name,
		Image:     cmp.Or(own.Result.TargetImage, own.Container.ImageRef),
		Digest:    own.Result.CandidateDigest,
	}
	stack := own.Container.Stack()
	plan := txn.Plan{
		ID: id, Host: host, Stacks: []string{stack}, Targets: []txn.Target{target}, SelfUpdate: true,
	}
	originals := []txn.Original{{ID: originalID, Running: true, Updated: true}}
	if err := r.journal.Begin(ctx, txn.OpenRecord{
		ID: id, Host: host, Stacks: plan.Stacks, State: txn.StateSnapshot, SelfUpdate: true,
		OwnContainer: originalName, Originals: originals,
	}); err != nil {
		return graph.HandoverTicket{}, err
	}
	ceiling := txn.NewCeiling(r.options.TransactionTimeout, r.clock.Now)
	ceiling.Begin()
	ctx = txn.WithCeiling(ctx, ceiling)
	r.logger.Info("snapshot", "phase", "snapshot", "txn", id)
	spec, err := r.engine.Inspect(ctx, originalID)
	if err != nil {
		return r.abort(ctx, id, own, err)
	}
	if err := SideBySideFailure(spec); err != nil {
		return r.abort(ctx, id, own, err)
	}
	if err := r.recreator.Reproducible(ctx, originalName); err != nil {
		return r.abort(ctx, id, own, err)
	}
	if err := r.pull(ctx, own.Container, target); err != nil {
		return r.abort(ctx, id, own, pullFailure(err))
	}
	snapshot, err := r.snapshot.Take(ctx, plan)
	if err != nil {
		return r.abort(ctx, id, own, snapshotFailure(err))
	}
	plan.Snapshot = snapshot
	if ceiling.Expired() {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateSnapshot, timeoutFailure())
	}
	r.logger.Info("handover", "phase", "handover", "txn", id)
	if err := r.journal.Advance(ctx, id, txn.StateHandover); err != nil {
		return graph.HandoverTicket{}, err
	}
	if err := handover.Write(r.files, r.stateDir, handover.Marker{
		Transaction: id, Container: originalID, Digest: target.Digest,
	}); err != nil {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateHandover, err)
	}
	suffix := "-parachute-" + id[len(id)-6:]
	if err := r.engine.Rename(ctx, originalID, originalName+suffix); err != nil {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateHandover, err)
	}
	replacement, err := r.recreator.Replacement(ctx, plan, originalID, originalName)
	if err != nil {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateHandover, err)
	}
	if err := r.engine.Start(ctx, replacement); err != nil {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateHandover, err)
	}
	own.Container.ID = replacement
	if ceiling.Expired() {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateHandover, timeoutFailure())
	}
	if err := r.verify(ctx, plan, originalName, ceiling); err != nil {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateHandover, err)
	}
	if ceiling.Expired() {
		return r.rollback(ctx, plan, own, originals, originalName, txn.StateHandover, timeoutFailure())
	}
	r.logger.Info("commit", "phase", "commit", "txn", id)
	if err := r.journal.Finish(ctx, id, txn.Outcome{
		State: txn.StateCommitted, Result: domain.StateUpdated, Superseded: originalID,
	}); err != nil {
		return graph.HandoverTicket{}, err
	}
	own.Result.State = domain.StateUpdated
	own.Result.Transaction = id
	own.Result.Snapshot = snapshot
	return graph.HandoverTicket{RemoveMarker: true}, nil
}

func (r *Runner) Finish(_ context.Context, ticket graph.HandoverTicket) error {
	if !ticket.RemoveMarker {
		return nil
	}
	return handover.Remove(r.files, r.stateDir)
}

func (r *Runner) abort(ctx context.Context, id string, own *pipeline.Candidate, err error) (graph.HandoverTicket, error) {
	final, reason, detail := classify(err, "engine_error")
	if final != domain.StateFailed {
		final = domain.StateFailed
	}
	own.Result = own.Result.Conclude(final, reason, detail)
	own.Result.Transaction = id
	_ = r.journal.Finish(ctx, id, txn.Outcome{State: txn.StateAborted, Result: domain.StateFailed, Reason: txn.Reason(reason)})
	return graph.HandoverTicket{}, nil
}

func (r *Runner) rollback(ctx context.Context, plan txn.Plan, own *pipeline.Candidate, originals []txn.Original, originalName string, from txn.State, cause error) (graph.HandoverTicket, error) {
	ctx = context.WithoutCancel(ctx)
	final, reason, detail := classify(cause, "engine_error")
	if from == txn.StateSnapshot {
		final = domain.StateFailed
	}
	r.logger.Info("rollback", "phase", "rollback", "txn", plan.ID)
	_ = handover.Remove(r.files, r.stateDir)
	if from != txn.StateSnapshot {
		if err := r.journal.Advance(ctx, plan.ID, txn.StateRollingBack); err != nil {
			return graph.HandoverTicket{}, err
		}
	}
	outcome, err := r.finishRollback(ctx, plan, originals, own, originalName, final, reason, detail)
	if from == txn.StateSnapshot && outcome.State == txn.StateRolledBack {
		outcome.State = txn.StateAborted
	}
	if finishErr := r.journal.Finish(ctx, plan.ID, outcome); finishErr != nil && err == nil {
		err = finishErr
	}
	return graph.HandoverTicket{}, err
}

func (r *Runner) finishRollback(ctx context.Context, plan txn.Plan, originals []txn.Original, own *pipeline.Candidate, originalName string, final domain.State, reason, detail string) (txn.Outcome, error) {
	suffix := "-parachute-" + plan.ID[len(plan.ID)-6:]
	var failure error
	if err := r.removeReplacements(ctx, plan.ID); err != nil {
		failure = err
	}
	for _, original := range originals {
		spec, err := r.engine.Inspect(ctx, original.ID)
		if err != nil {
			failure = firstError(failure, err)
			continue
		}
		if base, ok := strings.CutSuffix(spec.Name, suffix); ok && base == originalName {
			if err := r.engine.Rename(ctx, original.ID, originalName); err != nil {
				failure = firstError(failure, err)
			}
		}
	}
	if failure != nil || r.twoInstances(ctx, own.Container.Stack()) {
		if failure == nil {
			failure = fmt.Errorf("two Parachute containers remain")
		}
		r.logger.Error("rollback failed", "txn", plan.ID, "error", failure.Error())
		own.Result = own.Result.Conclude(domain.StateRollbackFailed, "restore_failed", joinDetail(detail, failure.Error()))
		own.Result.Transaction = plan.ID
		own.Result.Snapshot = plan.Snapshot
		return txn.Outcome{State: txn.StateRollbackFail, Result: domain.StateRollbackFailed, Reason: "restore_failed"}, failure
	}
	own.Container.ID = originals[0].ID
	own.Result = own.Result.Conclude(final, reason, detail)
	own.Result.Transaction = plan.ID
	own.Result.Snapshot = plan.Snapshot
	return txn.Outcome{State: txn.StateRolledBack, Result: final, Reason: txn.Reason(reason)}, nil
}

func (r *Runner) removeReplacements(ctx context.Context, transaction string) error {
	containers, err := r.engine.Containers(ctx)
	if err != nil {
		return err
	}
	var failure error
	for _, container := range containers {
		if container.Labels["parachute.transaction"] != transaction || container.Labels["parachute.helper"] == "true" {
			continue
		}
		if err := r.engine.Remove(ctx, container.ID, false, true); err != nil && failure == nil {
			failure = err
		}
	}
	return failure
}

func (r *Runner) twoInstances(ctx context.Context, _ string) bool {
	containers, err := r.engine.Containers(ctx)
	if err != nil {
		return false
	}
	count := 0
	for _, container := range containers {
		if container.Own {
			count++
		}
	}
	return count > 1
}

func (r *Runner) verify(ctx context.Context, plan txn.Plan, name string, ceiling *txn.Ceiling) error {
	if ceiling.Expired() {
		return timeoutFailure()
	}
	verifyCtx := ctx
	if r.options.TransactionTimeout > 0 {
		var cancel context.CancelFunc
		verifyCtx, cancel = context.WithTimeout(ctx, ceiling.Remaining())
		defer cancel()
	}
	verdict, err := r.verifier.Verify(verifyCtx, plan, name)
	if (err != nil || !verdict.Passed) && verifyCtx.Err() != nil && ctx.Err() == nil {
		return timeoutFailure()
	}
	if err != nil {
		return err
	}
	if verdict.Passed {
		return nil
	}
	detail := fmt.Sprintf("%s: %s", verdict.Check, verdict.Output)
	if verdict.Reason == verify.ReasonTimeout {
		detail += fmt.Sprintf("; deadline %ds expired; set the label parachute.verify.timeout for a slow-starting container", int(verdict.Deadline.Seconds()))
	}
	return recreate.Failure{State: string(domain.StateRolledBack), Reason: string(verdict.Reason), Detail: detail}
}

func (r *Runner) pull(ctx context.Context, container domain.Container, target txn.Target) error {
	reference := strings.Split(target.Image, "@")[0]
	if local, found, err := r.engine.Image(ctx, reference); err == nil && found && local.ID == target.Digest {
		return nil
	}
	parsed, err := registry.ParseReference(reference)
	if err != nil {
		return err
	}
	repository := parsed.Repository
	if parsed.Domain != "" && parsed.Domain != "docker.io" {
		repository = parsed.Domain + "/" + parsed.Repository
	}
	if err := r.engine.Pull(ctx, repository, target.Digest, r.auth(parsed)); err != nil {
		return err
	}
	return r.engine.Tag(ctx, repository+"@"+target.Digest, reference)
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
