package restoreverify

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/backup"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

const (
	resultPass            = "pass"
	resultFail            = "fail"
	reasonSnapshotInvalid = "snapshot_invalid"
	reasonRestoreFailed   = "restore_failed"
)

type Runner struct {
	files        FileSystem
	snapshotters map[string]Snapshotter
	verifier     Verifier
	ids          *txn.IDGenerator
	clock        Clock
	recoverer    *Recoverer
}

func NewRunner(files FileSystem, snapshotters map[string]Snapshotter, verifier Verifier, ids *txn.IDGenerator, clock Clock) *Runner {
	return &Runner{files: files, snapshotters: snapshotters, verifier: verifier, ids: ids, clock: clock, recoverer: NewRecoverer(files, clock)}
}

type Outcome struct {
	ExitCode int
	Err      error
}

func (r *Runner) Run(ctx context.Context, engines map[string]Engine, saved state.Document, options Options) Outcome {
	locker, err := AcquireLock(r.files, options.StateDir)
	if err != nil {
		if errors.Is(err, ErrLockHeld) {
			return Outcome{ExitCode: 2, Err: fmt.Errorf("another process holds %s", LockFileName())}
		}
		return Outcome{ExitCode: 2, Err: err}
	}
	defer func() { _ = locker.Release() }()

	_ = r.recoverer.Recover(ctx, engines, options.StateDir)

	targets := SelectTargets(saved, options)
	if len(targets) == 0 {
		return Outcome{ExitCode: 2, Err: errors.New("no snapshots to verify")}
	}

	runID, err := r.ids.New("rv")
	if err != nil {
		return Outcome{ExitCode: 2, Err: err}
	}

	document := Document{
		Format: Format, ID: runID, Open: true, Started: r.clock.Now().UTC(),
		Stacks: make([]StackRecord, len(targets)),
	}
	for index, target := range targets {
		document.Stacks[index] = StackRecord{
			Host: target.Host, Stack: target.Stack, Snapshot: target.Snapshot.ID,
		}
	}
	if err := WriteRecord(r.files, options.StateDir, document); err != nil {
		return Outcome{ExitCode: 2, Err: err}
	}

	allowed := []string{runID}
	failures := 0
	for index, target := range targets {
		engine := engines[target.Host]
		if engine == nil {
			failures++
			document.Stacks[index].Result = resultFail
			document.Stacks[index].Reason = reasonRestoreFailed
			continue
		}
		stackResult := r.verifyStack(ctx, engine, r.snapshotters[target.Host], target, runID)
		document.Stacks[index] = stackResult
		if stackResult.Result != resultPass {
			failures++
		}
		if engine != nil {
			_ = CleanupRuns(ctx, engine, allowed)
		}
	}

	document.Open = false
	document.Ended = r.clock.Now().UTC()
	if err := WriteRecord(r.files, options.StateDir, document); err != nil {
		return Outcome{ExitCode: 2, Err: err}
	}
	if failures > 0 {
		return Outcome{ExitCode: 1, Err: fmt.Errorf("restore verification failed for %d stack(s)", failures)}
	}
	return Outcome{ExitCode: 0}
}

func (r *Runner) verifyStack(ctx context.Context, engine Engine, snapshotter Snapshotter, target Target, runID string) StackRecord {
	record := StackRecord{
		Host: target.Host, Stack: target.Stack, Snapshot: target.Snapshot.ID,
		Result: resultFail, Reason: reasonRestoreFailed,
	}
	if snapshotter == nil || engine == nil {
		return record
	}
	endRestore := snapshotter.BeginRestoreVerify(runID)
	defer endRestore()
	defer func() { _ = CleanupRuns(ctx, engine, []string{runID}) }()

	manifest, reason := r.openSnapshot(ctx, snapshotter, target)
	if reason != "" {
		record.Reason = reason
		return record
	}
	stack := stackRun{engine: engine, snapshotter: snapshotter, target: target, manifest: manifest, runID: runID, suffix: Suffix(runID)}
	if reason := stack.restoreVolumes(ctx); reason != "" {
		record.Reason = reason
		return record
	}
	ids, reason := stack.createContainers(ctx)
	if reason != "" {
		record.Reason = reason
		return record
	}
	if reason := r.startAndVerify(ctx, stack, ids); reason != "" {
		record.Reason = reason
		return record
	}
	record.Result, record.Reason = resultPass, ""
	return record
}

func (r *Runner) openSnapshot(ctx context.Context, snapshotter Snapshotter, target Target) (backup.Manifest, string) {
	if err := snapshotter.ValidateTransaction(ctx, target.Snapshot.ID, target.Host, target.Stack, target.Snapshot.Transaction); err != nil {
		return backup.Manifest{}, reasonSnapshotInvalid
	}
	manifest, err := snapshotter.Manifest(ctx, target.Snapshot.ID)
	if err != nil {
		return backup.Manifest{}, reasonSnapshotInvalid
	}
	return manifest, ""
}

func (r *Runner) startAndVerify(ctx context.Context, stack stackRun, ids map[string]string) string {
	order := startOrder(stack.manifest)
	for _, name := range order {
		if id := ids[name]; id != "" {
			if err := stack.engine.Start(ctx, id); err != nil {
				return reasonRestoreFailed
			}
		}
	}
	plan := txn.Plan{ID: stack.runID, Host: stack.target.Host, Stacks: []string{stack.target.Stack}}
	for _, name := range order {
		id := ids[name]
		if id == "" {
			continue
		}
		verdict, err := r.verifier.Verify(ctx, plan, id)
		if err != nil {
			return reasonRestoreFailed
		}
		if !verdict.Passed {
			return cmp.Or(string(verdict.Reason), reasonRestoreFailed)
		}
	}
	return ""
}

func startOrder(manifest backup.Manifest) []string {
	containers := make([]domain.Container, len(manifest.Containers))
	names := make([]string, len(manifest.Containers))
	for index, spec := range manifest.Containers {
		labels := map[string]string{}
		links := []string{}
		networkMode := ""
		if spec.Config != nil {
			labels = spec.Config.Labels
		}
		if spec.HostConfig != nil {
			links = spec.HostConfig.Links
			networkMode = string(spec.HostConfig.NetworkMode)
		}
		containers[index] = domain.Container{Name: spec.Name, Labels: labels, Links: links, NetworkMode: networkMode}
		names[index] = spec.Name
	}
	index := graph.NewIndex(containers)
	edges := graph.AllEdges(index)
	return graph.StartSequence(names, graph.FilterEdges(edges, names))
}
