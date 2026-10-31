package app

import (
	"context"
	"log/slog"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type recoveryJournal interface {
	Open(ctx context.Context) ([]txn.OpenRecord, error)
	Advance(ctx context.Context, id string, next txn.State) error
	Finish(ctx context.Context, id string, outcome txn.Outcome) error
	Snapshots(host, stack string) []state.SnapshotRecord
}

type containerLister interface {
	Containers(ctx context.Context) ([]domain.Container, error)
}

type imageLookup interface {
	Image(ctx context.Context, reference string) (engine.Image, bool, error)
}

type RecoveryHost struct {
	Lister   containerLister
	Images   imageLookup
	Finisher graph.Rollbacker
	Retain   func(ctx context.Context, host, stack string) error
}

type Recovery struct {
	hosts   map[string]RecoveryHost
	journal recoveryJournal
	logger  *slog.Logger
	results []domain.Result
}

func NewRecovery(hosts map[string]RecoveryHost, journal recoveryJournal, logger *slog.Logger) *Recovery {
	return &Recovery{hosts: hosts, journal: journal, logger: logger}
}

func (r *Recovery) Recover(ctx context.Context) {
	open, err := r.journal.Open(ctx)
	if err != nil {
		r.logger.Error("cannot read open transactions", "error", err.Error())
		return
	}
	for _, record := range open {
		host, known := r.hosts[record.Host]
		if !known || record.SelfUpdate || record.State == txn.StateHandover {
			continue
		}
		r.recover(ctx, host, record)
	}
}

func (r *Recovery) recover(ctx context.Context, host RecoveryHost, record txn.OpenRecord) {
	containers, err := host.Lister.Containers(ctx)
	if err != nil {
		r.logger.Error("cannot recover transaction", "txn", record.ID, "error", err.Error())
		return
	}
	r.logger.Info("recovering interrupted transaction", "txn", record.ID, "state", string(record.State))
	aborted := record.State == txn.StateSnapshot
	final := domain.StateRolledBack
	if aborted {
		final = domain.StateFailed
	} else if record.State != txn.StateRollingBack {
		if err := r.journal.Advance(ctx, record.ID, txn.StateRollingBack); err != nil {
			r.logger.Error("cannot recover transaction", "txn", record.ID, "error", err.Error())
			return
		}
	}
	candidates := recoveryCandidates(ctx, host, record, containers)
	plan := txn.Plan{ID: record.ID, Host: record.Host, Stacks: record.Stacks, Snapshot: r.snapshotOf(record)}
	outcome, err := host.Finisher.FinishRollback(ctx, plan, record.Originals, candidates, final, "interrupted", "")
	if err != nil {
		r.logger.Error("cannot recover transaction", "txn", record.ID, "error", err.Error())
		return
	}
	if aborted && outcome.State == txn.StateRolledBack {
		outcome.State = txn.StateAborted
	} else if outcome.State == txn.StateRolledBack && host.Retain != nil {
		for _, stack := range record.Stacks {
			if err := host.Retain(ctx, record.Host, stack); err != nil {
				r.logger.Warn("cannot apply snapshot retention", "txn", record.ID, "stack", stack, "error", err.Error())
			}
		}
	}
	if err := r.journal.Finish(ctx, record.ID, outcome); err != nil {
		r.logger.Error("cannot record recovered transaction", "txn", record.ID, "error", err.Error())
	}
	for _, candidate := range candidates {
		r.results = append(r.results, candidate.Result)
	}
}

func (r *Recovery) snapshotOf(record txn.OpenRecord) string {
	if len(record.Stacks) == 0 {
		return record.Snapshot
	}
	for _, snapshot := range r.journal.Snapshots(record.Host, record.Stacks[0]) {
		if snapshot.Transaction == record.ID {
			return snapshot.ID
		}
	}
	return record.Snapshot
}

func recoveryCandidates(ctx context.Context, host RecoveryHost, record txn.OpenRecord, containers []domain.Container) []*pipeline.Candidate {
	suffix := "-parachute-" + record.ID[len(record.ID)-6:]
	digests := replacementDigests(ctx, host.Images, record.ID, containers)
	var candidates []*pipeline.Candidate
	for _, original := range record.Originals {
		if !original.Updated {
			continue
		}
		container := domain.Container{ID: original.ID, Name: original.ID}
		for _, listed := range containers {
			if listed.ID == original.ID {
				container = listed
				container.Name = strings.TrimSuffix(listed.Name, suffix)
			}
		}
		result := domain.NewResult(record.Host, container)
		result.CandidateDigest = digests[container.Name]
		candidates = append(candidates, &pipeline.Candidate{Container: container, Result: result})
	}
	return candidates
}

func replacementDigests(ctx context.Context, images imageLookup, transaction string, containers []domain.Container) map[string]string {
	digests := map[string]string{}
	for _, listed := range containers {
		if listed.Labels["parachute.transaction"] != transaction || listed.Labels["parachute.helper"] == "true" {
			continue
		}
		digest := listed.ImageID
		if images != nil {
			if image, found, err := images.Image(ctx, listed.ImageID); err == nil && found && len(image.Digests) > 0 {
				digest = image.Digests[0]
			}
		}
		digests[listed.Name] = digest
	}
	return digests
}

func (r *Recovery) Run(_ context.Context, cycle *pipeline.Cycle) error {
	for _, host := range cycle.Hosts {
		for _, result := range r.results {
			if result.Host != host.Host.Name {
				continue
			}
			host.Candidates = withRecovered(host.Candidates, result)
		}
	}
	r.results = nil
	return nil
}

func withRecovered(candidates []*pipeline.Candidate, result domain.Result) []*pipeline.Candidate {
	for _, candidate := range candidates {
		if candidate.Container.Name == result.Container {
			candidate.Result = result
			return candidates
		}
	}
	container := domain.Container{ID: result.ContainerID, Name: result.Container}
	return append(candidates, &pipeline.Candidate{Container: container, Result: result})
}

func RecoverInterruptedSelfHandover(ctx context.Context, finisher *RollbackFinisher, journal recoveryJournal, record txn.OpenRecord) (txn.Outcome, []domain.Result, error) {
	outcome, candidates, err := finisher.RollbackSelfHandover(ctx, record)
	if err != nil {
		return txn.Outcome{}, nil, err
	}
	if err := journal.Finish(ctx, record.ID, outcome); err != nil {
		return txn.Outcome{}, nil, err
	}
	var results []domain.Result
	for _, candidate := range candidates {
		results = append(results, candidate.Result)
	}
	return outcome, results, nil
}
