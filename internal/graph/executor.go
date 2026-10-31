package graph

import (
	"cmp"
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/recreate"
	"github.com/KazzyAPI/parachute/internal/registry"
	"github.com/KazzyAPI/parachute/internal/txn"
)

const (
	dependencyWait = 2 * time.Minute
	dependencyPoll = 250 * time.Millisecond
)

type Engine interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Stop(ctx context.Context, id, signal string, timeout time.Duration) error
	Start(ctx context.Context, id string) error
	Rename(ctx context.Context, id, name string) error
	Remove(ctx context.Context, id string, anonymousVolumes, force bool) error
	RemoveImage(ctx context.Context, reference string) error
	Image(ctx context.Context, reference string) (engine.Image, bool, error)
	Pull(ctx context.Context, repository, digest string, auth engine.RegistryAuth) error
	Tag(ctx context.Context, source, target string) error
	Events(ctx context.Context, since time.Time, filters map[string][]string) (<-chan engine.Event, <-chan error)
}

type Rollbacker interface {
	FinishRollback(ctx context.Context, plan txn.Plan, originals []txn.Original, candidates []*pipeline.Candidate, final domain.State, reason, detail string) (txn.Outcome, error)
}

type Clock interface {
	Now() time.Time
}

type Executor struct {
	builder    *Builder
	engine     Engine
	journal    txn.Journal
	snapshot   txn.Snapshotter
	recreator  txn.Recreator
	verifier   txn.Verifier
	rollbacker Rollbacker
	hooks      txn.Hooks
	selfRunner SelfRunner
	ids        *txn.IDGenerator
	clock      Clock
	logger     *slog.Logger
	options    Options
	auth       func(registry.Reference) engine.RegistryAuth
}

type Options struct {
	StopTimeout        time.Duration
	Rolling            bool
	Cleanup            bool
	RemoveVolumes      bool
	Retention          int
	TransactionTimeout time.Duration
	NoRestart          bool
	ReviveStopped      bool
}

func NewExecutor(
	builder *Builder,
	engine Engine,
	journal txn.Journal,
	snapshot txn.Snapshotter,
	recreator txn.Recreator,
	verifier txn.Verifier,
	rollbacker Rollbacker,
	hooks txn.Hooks,
	selfRunner SelfRunner,
	ids *txn.IDGenerator,
	clock Clock,
	logger *slog.Logger,
	options Options,
	auth func(registry.Reference) engine.RegistryAuth,
) *Executor {
	return &Executor{
		builder: builder, engine: engine, journal: journal, snapshot: snapshot, recreator: recreator,
		verifier: verifier, rollbacker: rollbacker, hooks: hooks, selfRunner: selfRunner, ids: ids, clock: clock,
		logger: logger, options: options, auth: auth,
	}
}

func (e *Executor) Run(ctx context.Context, host string, containers []domain.Container, candidates []*pipeline.Candidate, own string) (HandoverTicket, error) {
	if len(candidates) == 0 {
		return HandoverTicket{}, nil
	}
	index := NewIndex(containers)
	targets := map[string]txn.Target{}
	for _, candidate := range candidates {
		if candidate.Result.State != domain.StateWouldUpdate {
			continue
		}
		targets[candidate.Container.Name] = txn.Target{
			Container: candidate.Container.Name,
			Image:     cmp.Or(candidate.Result.TargetImage, candidate.Container.ImageRef),
			Digest:    candidate.Result.CandidateDigest,
		}
	}
	plan := e.builder.Plan(index, targets, own)
	for _, cycle := range plan.Cycles {
		for _, name := range cycle.Containers {
			e.failCandidate(candidates, name, "dependency_cycle", strings.Join(cycle.Containers, ", "))
		}
	}
	for _, group := range plan.Groups {
		if err := e.runGroup(ctx, host, index, group, candidates); err != nil {
			return HandoverTicket{}, err
		}
	}
	return e.selfRunner.Handover(ctx, host, ownCandidate(candidates, own))
}

func ownCandidate(candidates []*pipeline.Candidate, own string) *pipeline.Candidate {
	for _, candidate := range candidates {
		if own != "" && (candidate.Container.Name == own || candidate.Container.ID == own) {
			return candidate
		}
	}
	for _, candidate := range candidates {
		if candidate.Container.Own {
			return candidate
		}
	}
	return nil
}

func (e *Executor) runGroup(ctx context.Context, host string, index Index, group Group, candidates []*pipeline.Candidate) error {
	if len(group.Targets) == 0 {
		return nil
	}
	id, err := e.ids.New("txn")
	if err != nil {
		return err
	}
	names := MemberNames(group.Members)
	edges := FilterEdges(AllEdges(index), names)
	affected := affectedCandidates(group, candidates)
	plan := txn.Plan{ID: id, Host: host, Stacks: group.Stacks, Targets: group.Targets}
	originals := originalsOf(group, StartSequence(names, edges))
	if err := e.journal.Begin(ctx, txn.OpenRecord{ID: id, Host: host, Stacks: group.Stacks, State: txn.StateSnapshot, Originals: originals}); err != nil {
		return err
	}
	ceiling := txn.NewCeiling(e.options.TransactionTimeout, e.clock.Now)
	ceiling.Begin()
	ctx = txn.WithCeiling(ctx, ceiling)
	e.logger.Info("snapshot", "phase", "snapshot", "txn", id)
	if err := e.prepare(ctx, plan, group); err != nil {
		e.abort(ctx, id, affected, err)
		return nil
	}
	if _, err := e.snapshot.Estimate(ctx, plan); err != nil {
		e.abort(ctx, id, affected, snapshotFailure(err))
		return nil
	}
	if err := e.runPreUpdateHooks(ctx, group, edges, ceiling); err != nil {
		if errors.Is(err, errDeferred) {
			e.abortDeferred(ctx, id, affected)
			return nil
		}
		_, reason, _ := classify(err, "")
		if reason == "transaction_timeout" {
			return e.rollback(ctx, plan, affected, originals, txn.StateSnapshot, err)
		}
		e.abortPreUpdateHook(ctx, id, affected, err)
		return nil
	}
	if ceiling.Expired() {
		return e.rollback(ctx, plan, affected, originals, txn.StateSnapshot, timeoutFailure())
	}
	snapshot, err := e.snapshot.Take(ctx, plan)
	if err != nil {
		e.abort(ctx, id, affected, snapshotFailure(err))
		return nil
	}
	plan.Snapshot = snapshot
	if ceiling.Expired() {
		return e.rollback(ctx, plan, affected, originals, txn.StateSnapshot, timeoutFailure())
	}
	if err := e.journal.Advance(ctx, id, txn.StateUpdate); err != nil {
		return err
	}
	e.logger.Info("update", "phase", "update", "txn", id)
	if e.options.Rolling {
		e.logger.Info("rolling restart groups", "txn", id, "groups", len(RollingGroups(names, edges)))
		err = e.rollingUpdate(ctx, plan, group, edges, candidates, ceiling)
	} else {
		err = e.batchUpdate(ctx, plan, group, edges, candidates)
	}
	if err == nil && ceiling.Expired() {
		err = timeoutFailure()
	}
	if err != nil {
		return e.rollback(ctx, plan, affected, originals, txn.StateUpdate, err)
	}
	if err := e.journal.Advance(ctx, id, txn.StateVerify); err != nil {
		return err
	}
	e.logger.Info("verify", "phase", "verify", "txn", id)
	for _, member := range group.Members {
		if !member.Update && !member.Recreate {
			continue
		}
		if err := e.verifyMember(ctx, plan, member.Container.Name, ceiling); err != nil {
			return e.rollback(ctx, plan, affected, originals, txn.StateVerify, err)
		}
	}
	if ceiling.Expired() {
		return e.rollback(ctx, plan, affected, originals, txn.StateVerify, timeoutFailure())
	}
	e.logger.Info("commit", "phase", "commit", "txn", id)
	if err := e.journal.Finish(ctx, id, txn.Outcome{State: txn.StateCommitted, Result: domain.StateUpdated, Reason: txn.Reason("")}); err != nil {
		return err
	}
	e.removeOriginals(ctx, group)
	for _, stack := range group.Stacks {
		_ = e.snapshot.Retain(ctx, host, stack, e.options.Retention)
	}
	skipped, _ := e.snapshot.SkippedDestinations(ctx, snapshot)
	for _, member := range group.Members {
		if member.Update {
			e.markUpdated(candidates, member.Container.Name, id, snapshot, skipped[member.Container.Name])
		}
	}
	for _, member := range group.Members {
		if !member.Update && member.Recreate {
			e.markUpdated(candidates, member.Container.Name, id, snapshot, skipped[member.Container.Name])
		}
	}
	for _, member := range group.Members {
		if !member.Update && !member.Recreate {
			e.joinTransaction(candidates, member.Container.Name, id)
		}
	}
	e.runPostChecks(ctx, transactionCandidates(group, candidates))
	return nil
}

func (e *Executor) joinTransaction(candidates []*pipeline.Candidate, name, id string) {
	for _, candidate := range candidates {
		if candidate.Container.Name == name {
			candidate.Result.Transaction = id
		}
	}
}

func (e *Executor) prepare(ctx context.Context, plan txn.Plan, group Group) error {
	for _, member := range group.Members {
		if member.Update {
			if err := e.pull(ctx, member.Container, plan.Targets); err != nil {
				return pullFailure(err)
			}
		}
	}
	for _, member := range group.Members {
		if member.Update || member.Recreate {
			if err := e.recreator.Reproducible(ctx, member.Container.Name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) verifyMember(ctx context.Context, plan txn.Plan, name string, ceiling *txn.Ceiling) error {
	if ceiling.Expired() {
		return timeoutFailure()
	}
	verifyCtx := ctx
	if e.options.TransactionTimeout > 0 {
		var cancel context.CancelFunc
		verifyCtx, cancel = context.WithTimeout(ctx, ceiling.Remaining())
		defer cancel()
	}
	verdict, err := e.verifier.Verify(verifyCtx, plan, name)
	if (err != nil || !verdict.Passed) && verifyCtx.Err() != nil && ctx.Err() == nil {
		return timeoutFailure()
	}
	if err != nil {
		return err
	}
	if verdict.Passed {
		return nil
	}
	return recreate.Failure{State: string(domain.StateRolledBack), Reason: string(verdict.Reason), Detail: verdictDetail(verdict)}
}

func (e *Executor) abort(ctx context.Context, id string, affected []*pipeline.Candidate, err error) {
	_, reason, detail := classify(err, "engine_error")
	for _, candidate := range affected {
		candidate.Result = candidate.Result.Conclude(domain.StateFailed, reason, detail)
		candidate.Result.Transaction = id
	}
	_ = e.journal.Finish(ctx, id, txn.Outcome{State: txn.StateAborted, Result: domain.StateFailed, Reason: txn.Reason(reason)})
}

func (e *Executor) rollback(ctx context.Context, plan txn.Plan, affected []*pipeline.Candidate, originals []txn.Original, from txn.State, cause error) error {
	ctx = context.WithoutCancel(ctx)
	final, reason, detail := classify(cause, "engine_error")
	if from == txn.StateSnapshot {
		final = domain.StateFailed
	}
	e.logger.Info("rollback", "phase", "rollback", "txn", plan.ID)
	if from != txn.StateSnapshot {
		if err := e.journal.Advance(ctx, plan.ID, txn.StateRollingBack); err != nil {
			e.logger.Error("cannot record rolling_back; continuing rollback", "txn", plan.ID, "error", err.Error())
		}
	}
	outcome, err := e.rollbacker.FinishRollback(ctx, plan, originals, affected, final, reason, detail)
	if from == txn.StateSnapshot && outcome.State == txn.StateRolledBack {
		outcome.State = txn.StateAborted
	}
	if finishErr := e.journal.Finish(ctx, plan.ID, outcome); finishErr != nil && err == nil {
		err = finishErr
	}
	if outcome.State == txn.StateRolledBack {
		for _, stack := range plan.Stacks {
			_ = e.snapshot.Retain(ctx, plan.Host, stack, e.options.Retention)
		}
	}
	e.runPostChecks(ctx, affected)
	return err
}

func affectedCandidates(group Group, candidates []*pipeline.Candidate) []*pipeline.Candidate {
	var affected []*pipeline.Candidate
	for _, member := range group.Members {
		if !member.Update && !member.Recreate {
			continue
		}
		for _, candidate := range candidates {
			if candidate.Container.Name == member.Container.Name {
				affected = append(affected, candidate)
			}
		}
	}
	return affected
}

func originalsOf(group Group, startOrder []string) []txn.Original {
	var originals []txn.Original
	for _, name := range startOrder {
		member := memberOf(group, name)
		if member == nil {
			continue
		}
		running := wasRunning(*member)
		originals = append(originals, txn.Original{ID: member.Container.ID, Running: running, Updated: member.Update || member.Recreate})
	}
	return originals
}

func (e *Executor) removeOriginals(ctx context.Context, group Group) {
	for _, member := range group.Members {
		if !member.Update && !member.Recreate {
			continue
		}
		_ = e.engine.Remove(ctx, member.Container.ID, e.options.RemoveVolumes, true)
		if member.Update && e.options.Cleanup {
			_ = e.engine.RemoveImage(ctx, member.Container.ImageID)
		}
	}
}

func (e *Executor) batchUpdate(ctx context.Context, plan txn.Plan, group Group, edges []Edge, candidates []*pipeline.Candidate) error {
	names := MemberNames(group.Members)
	for _, name := range StopOrder(names, edges) {
		if err := e.stopContainer(ctx, name, indexName(group, name)); err != nil {
			return err
		}
	}
	if err := e.renameOriginals(ctx, plan, group, names); err != nil {
		return err
	}
	return e.startAll(ctx, plan, group, names, edges, candidates, nil)
}

func (e *Executor) rollingUpdate(ctx context.Context, plan txn.Plan, group Group, edges []Edge, candidates []*pipeline.Candidate, ceiling *txn.Ceiling) error {
	names := MemberNames(group.Members)
	for _, batch := range RollingGroups(names, edges) {
		batchEdges := FilterEdges(edges, batch)
		for _, name := range StopOrder(batch, batchEdges) {
			if err := e.stopContainer(ctx, name, indexName(group, name)); err != nil {
				return err
			}
		}
		if err := e.renameOriginals(ctx, plan, group, batch); err != nil {
			return err
		}
		verify := func(name string) error { return e.verifyMember(ctx, plan, name, ceiling) }
		if err := e.startAll(ctx, plan, group, batch, batchEdges, candidates, verify); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) renameOriginals(ctx context.Context, plan txn.Plan, group Group, names []string) error {
	suffix := plan.ID[len(plan.ID)-6:]
	for _, name := range names {
		member := memberOf(group, name)
		if member == nil || !member.Update && !member.Recreate {
			continue
		}
		if err := e.engine.Rename(ctx, member.Container.ID, member.Container.Name+"-parachute-"+suffix); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) startAll(ctx context.Context, plan txn.Plan, group Group, names []string, edges []Edge, candidates []*pipeline.Candidate, verify func(string) error) error {
	running := map[string]string{}
	for _, name := range StartSequence(names, edges) {
		member := memberOf(group, name)
		if member == nil {
			continue
		}
		if err := e.awaitDependencies(ctx, name, edges, running); err != nil {
			return err
		}
		if !member.Update && !member.Recreate {
			if !wasRunning(*member) {
				continue
			}
			if err := e.engine.Start(ctx, member.Container.ID); err != nil {
				return err
			}
			running[name] = member.Container.ID
			continue
		}
		replacement, err := e.recreator.Replacement(ctx, plan, member.Container.ID, member.Container.Name)
		if err != nil {
			return err
		}
		if err := e.startReplacement(ctx, *member, replacement); err != nil {
			return err
		}
		e.setReplacement(candidates, member.Container.Name, replacement)
		for _, candidate := range candidates {
			if candidate.Container.Name == member.Container.Name {
				e.runPostUpdateHook(ctx, candidate.Container)
				break
			}
		}
		running[name] = replacement
		if verify != nil {
			if err := verify(name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) awaitDependencies(ctx context.Context, name string, edges []Edge, running map[string]string) error {
	for _, edge := range edges {
		id, ok := running[edge.Dependency]
		if edge.Dependent != name || edge.Condition == ConditionStarted || !ok {
			continue
		}
		if err := e.awaitCondition(ctx, id, edge.Condition); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) awaitCondition(ctx context.Context, id string, condition Condition) error {
	ctx, cancel := context.WithTimeout(ctx, dependencyWait)
	defer cancel()
	for {
		spec, err := e.engine.Inspect(ctx, id)
		if err != nil {
			return dependencyFailure(ctx, err)
		}
		if condition == ConditionHealthy && spec.HealthStatus == "healthy" ||
			condition == ConditionExitOK && spec.Status == "exited" && spec.ExitCode == 0 {
			return nil
		}
		if condition == ConditionHealthy && spec.HealthStatus == "unhealthy" ||
			condition == ConditionExitOK && spec.Status == "exited" {
			return recreate.Failure{State: string(domain.StateRolledBack), Reason: "verify_failed", Detail: "dependency " + spec.Name + " did not reach its start condition"}
		}
		select {
		case <-ctx.Done():
			return dependencyFailure(ctx, ctx.Err())
		case <-time.After(dependencyPoll):
		}
	}
}

func dependencyFailure(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return recreate.Failure{State: string(domain.StateRolledBack), Reason: "verify_timeout", Detail: "dependency start condition"}
	}
	return err
}

func (e *Executor) pull(ctx context.Context, container domain.Container, targets []txn.Target) error {
	for _, target := range targets {
		if target.Container != container.Name {
			continue
		}
		reference := strings.Split(target.Image, "@")[0]
		if local, found, err := e.engine.Image(ctx, reference); err == nil && found && local.ID == target.Digest {
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
		if err := e.engine.Pull(ctx, repository, target.Digest, e.auth(parsed)); err != nil {
			return err
		}
		return e.engine.Tag(ctx, repository+"@"+target.Digest, reference)
	}
	return nil
}

func (e *Executor) stopContainer(ctx context.Context, name string, member *Member) error {
	if member == nil {
		return nil
	}
	if !wasRunning(*member) {
		return nil
	}
	return e.engine.Stop(ctx, member.Container.ID, "", e.options.StopTimeout)
}

func wasRunning(member Member) bool {
	return member.Container.State == "running" || member.Container.State == "restarting"
}

func (e *Executor) startReplacement(ctx context.Context, member Member, replacement string) error {
	if e.options.NoRestart || !wasRunning(member) && !e.options.ReviveStopped {
		return nil
	}
	return e.engine.Start(ctx, replacement)
}

func indexName(group Group, name string) *Member {
	return memberOf(group, name)
}

func memberOf(group Group, name string) *Member {
	for i, member := range group.Members {
		if member.Container.Name == name {
			return &group.Members[i]
		}
	}
	return nil
}

func (e *Executor) failCandidate(candidates []*pipeline.Candidate, name, reason, detail string) {
	for _, candidate := range candidates {
		if candidate.Container.Name == name {
			candidate.Result = candidate.Result.Conclude(domain.StateFailed, reason, detail)
		}
	}
}

func (e *Executor) markUpdated(candidates []*pipeline.Candidate, name, id, snapshot string, skipped []string) {
	for _, candidate := range candidates {
		if candidate.Container.Name == name {
			candidate.Result.State = domain.StateUpdated
			candidate.Result.Transaction = id
			candidate.Result.Snapshot = snapshot
			for _, destination := range skipped {
				candidate.Result.Detail = strings.TrimPrefix(candidate.Result.Detail+"; skipped system mount "+destination, "; ")
			}
		}
	}
}

func (e *Executor) setReplacement(candidates []*pipeline.Candidate, name, replacement string) {
	for _, candidate := range candidates {
		if candidate.Container.Name == name {
			candidate.Container.ID = replacement
		}
	}
}
