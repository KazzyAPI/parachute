package graph

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/registry"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type Engine interface {
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Stop(ctx context.Context, id, signal string, timeout time.Duration) error
	Start(ctx context.Context, id string) error
	Rename(ctx context.Context, id, name string) error
	Remove(ctx context.Context, id string, anonymousVolumes, force bool) error
	RemoveImage(ctx context.Context, reference string) error
	Pull(ctx context.Context, repository, digest string, auth engine.RegistryAuth) error
	Tag(ctx context.Context, source, target string) error
	Events(ctx context.Context, since time.Time, filters map[string][]string) (<-chan engine.Event, <-chan error)
}

type Clock interface {
	Now() time.Time
}

type Executor struct {
	builder   *Builder
	engine    Engine
	journal   txn.Journal
	snapshot  txn.Snapshotter
	recreator txn.Recreator
	verifier  txn.Verifier
	ids       *txn.IDGenerator
	clock     Clock
	logger    *slog.Logger
	options   Options
	auth      engine.RegistryAuth
}

type Options struct {
	StopTimeout   time.Duration
	Rolling       bool
	Cleanup       bool
	RemoveVolumes bool
	Retention     int
}

func NewExecutor(
	builder *Builder,
	engine Engine,
	journal txn.Journal,
	snapshot txn.Snapshotter,
	recreator txn.Recreator,
	verifier txn.Verifier,
	ids *txn.IDGenerator,
	clock Clock,
	logger *slog.Logger,
	options Options,
	auth engine.RegistryAuth,
) *Executor {
	return &Executor{
		builder: builder, engine: engine, journal: journal, snapshot: snapshot, recreator: recreator,
		verifier: verifier, ids: ids, clock: clock, logger: logger, options: options, auth: auth,
	}
}

func (e *Executor) Run(ctx context.Context, host string, containers []domain.Container, candidates []*pipeline.Candidate, own string) error {
	if len(candidates) == 0 {
		return nil
	}
	index := NewIndex(containers)
	targets := map[string]txn.Target{}
	for _, candidate := range candidates {
		if candidate.Result.State != domain.StateWouldUpdate {
			continue
		}
		targets[candidate.Container.Name] = txn.Target{
			Container: candidate.Container.Name,
			Image:     candidate.Container.ImageRef,
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
			return err
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
	plan := txn.Plan{ID: id, Host: host, Stacks: group.Stacks, Targets: group.Targets}
	if err := e.journal.Begin(ctx, txn.OpenRecord{ID: id, Host: host, Stacks: group.Stacks, State: txn.StateSnapshot}); err != nil {
		return err
	}
	e.logger.Info("snapshot", "phase", "snapshot", "txn", id)
	snapshot, err := e.snapshot.Take(ctx, plan)
	if err != nil {
		_ = e.journal.Finish(ctx, id, txn.Outcome{State: txn.StateAborted, Result: domain.StateFailed, Reason: txn.Reason("snapshot_failed")})
		return err
	}
	plan.Snapshot = snapshot
	if err := e.journal.Advance(ctx, id, txn.StateUpdate); err != nil {
		return err
	}
	e.logger.Info("update", "phase", "update", "txn", id)
	names := MemberNames(group.Members)
	edges := FilterEdges(AllEdges(index), names)
	if e.options.Rolling {
		e.logger.Info("rolling restart groups", "txn", id, "groups", len(RollingGroups(names, edges)))
		if err := e.rollingUpdate(ctx, plan, group, edges, candidates, id); err != nil {
			return err
		}
	} else if err := e.batchUpdate(ctx, plan, group, edges, candidates, id); err != nil {
		return err
	}
	if err := e.journal.Advance(ctx, id, txn.StateVerify); err != nil {
		return err
	}
	e.logger.Info("verify", "phase", "verify", "txn", id)
	for _, member := range group.Members {
		if !member.Update && !member.Recreate {
			continue
		}
		verdict, err := e.verifier.Verify(ctx, plan, member.Container.Name)
		if err != nil || !verdict.Passed {
			e.logger.Info("rollback", "phase", "rollback", "txn", id)
			_ = e.journal.Finish(ctx, id, txn.Outcome{State: txn.StateRolledBack, Result: domain.StateRolledBack, Reason: verdict.Reason})
			return err
		}
	}
	e.logger.Info("commit", "phase", "commit", "txn", id)
	if err := e.journal.Finish(ctx, id, txn.Outcome{State: txn.StateCommitted, Result: domain.StateUpdated, Reason: txn.Reason("")}); err != nil {
		return err
	}
	e.removeOriginals(ctx, group)
	for _, stack := range group.Stacks {
		_ = e.snapshot.Retain(ctx, host, stack, e.options.Retention)
	}
	for _, member := range group.Members {
		if member.Update {
			e.markUpdated(candidates, member.Container.Name, id, snapshot)
		}
	}
	for _, member := range group.Members {
		if !member.Update && member.Recreate {
			e.markUpdated(candidates, member.Container.Name, id, snapshot)
		}
	}
	return nil
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

func (e *Executor) batchUpdate(ctx context.Context, plan txn.Plan, group Group, edges []Edge, candidates []*pipeline.Candidate, id string) error {
	names := MemberNames(group.Members)
	for _, name := range StopOrder(names, edges) {
		if err := e.stopContainer(ctx, name, indexName(group, name)); err != nil {
			return err
		}
	}
	suffix := plan.ID[len(plan.ID)-6:]
	for _, member := range group.Members {
		if member.Update || member.Recreate {
			if err := e.engine.Rename(ctx, member.Container.ID, member.Container.Name+"-parachute-"+suffix); err != nil {
				return err
			}
		}
	}
	for _, member := range group.Members {
		if !member.Update && !member.Recreate {
			continue
		}
		if member.Update {
			if err := e.pull(ctx, member.Container, plan.Targets); err != nil {
				return err
			}
		}
		replacement, err := e.recreator.Replacement(ctx, plan, member.Container.Name)
		if err != nil {
			return err
		}
		if err := e.engine.Start(ctx, replacement); err != nil {
			return err
		}
		e.setReplacement(candidates, member.Container.Name, replacement)
	}
	for _, name := range StartSequence(names, edges) {
		if err := e.startMember(ctx, group, name); err != nil {
			return err
		}
	}
	return nil
}

func (e *Executor) rollingUpdate(ctx context.Context, plan txn.Plan, group Group, edges []Edge, candidates []*pipeline.Candidate, id string) error {
	names := MemberNames(group.Members)
	for _, batch := range RollingGroups(names, edges) {
		batchEdges := FilterEdges(edges, batch)
		for _, name := range StopOrder(batch, batchEdges) {
			if err := e.stopContainer(ctx, name, indexName(group, name)); err != nil {
				return err
			}
		}
		for _, name := range batch {
			member := memberOf(group, name)
			if member == nil || (!member.Update && !member.Recreate) {
				continue
			}
			if member.Update {
				if err := e.pull(ctx, member.Container, plan.Targets); err != nil {
					return err
				}
			}
			replacement, err := e.recreator.Replacement(ctx, plan, member.Container.Name)
			if err != nil {
				return err
			}
			if err := e.engine.Start(ctx, replacement); err != nil {
				return err
			}
			e.setReplacement(candidates, member.Container.Name, replacement)
			verdict, err := e.verifier.Verify(ctx, plan, member.Container.Name)
			if err != nil || !verdict.Passed {
				return err
			}
		}
		for _, name := range StartSequence(batch, batchEdges) {
			if err := e.startMember(ctx, group, name); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) pull(ctx context.Context, container domain.Container, targets []txn.Target) error {
	for _, target := range targets {
		if target.Container != container.Name {
			continue
		}
		reference := strings.Split(container.ImageRef, "@")[0]
		parsed, err := registry.ParseReference(reference)
		if err != nil {
			return err
		}
		repository := parsed.Repository
		if parsed.Domain != "" && parsed.Domain != "docker.io" {
			repository = parsed.Domain + "/" + parsed.Repository
		}
		if err := e.engine.Pull(ctx, repository, target.Digest, e.auth); err != nil {
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
	if member.Container.State != "running" && member.Container.State != "restarting" {
		return nil
	}
	return e.engine.Stop(ctx, member.Container.ID, "", e.options.StopTimeout)
}

func (e *Executor) startMember(ctx context.Context, group Group, name string) error {
	member := memberOf(group, name)
	if member == nil || member.Update || member.Recreate {
		return nil
	}
	return e.engine.Start(ctx, member.Container.ID)
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

func (e *Executor) markUpdated(candidates []*pipeline.Candidate, name, id, snapshot string) {
	for _, candidate := range candidates {
		if candidate.Container.Name == name {
			candidate.Result.State = domain.StateUpdated
			candidate.Result.Transaction = id
			candidate.Result.Snapshot = snapshot
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
