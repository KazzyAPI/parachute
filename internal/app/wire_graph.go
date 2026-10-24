package app

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/graph"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func (c *Container) graphStage(store *state.Store, connected []connectedHost) pipeline.Stage {
	var docker *engine.Docker
	for _, host := range connected {
		if host.docker != nil && host.err == nil {
			docker = host.docker
			break
		}
	}
	if docker == nil {
		return graph.NewStage(nil, "")
	}
	journal := txnJournal{store: store, now: c.deps.Clock.Now}
	snapshot := txnSnapshot{}
	recreator := txnRecreator{engine: docker}
	verifier := txnVerifier{}
	executor := graph.NewExecutor(
		graph.NewBuilder(),
		docker,
		journal,
		snapshot,
		recreator,
		verifier,
		txn.NewIDGenerator(c.deps.Clock.Now),
		c.deps.Clock,
		c.logger,
		c.cfg.Update.StopTimeout,
		c.cfg.Update.RollingRestart,
		"",
	)
	return graph.NewStage(executor, "")
}

type txnJournal struct {
	store *state.Store
	now   func() time.Time
}

func (j txnJournal) Begin(ctx context.Context, record txn.OpenRecord) error {
	return j.store.Update(func(document *state.Document) error {
		document.Transactions = append(document.Transactions, state.TransactionRecord{
			ID: record.ID, Host: record.Host, Stacks: record.Stacks, State: string(record.State), Snapshot: record.Snapshot,
			SelfUpdate: record.SelfUpdate, Time: j.now(),
		})
		return nil
	})
}

func (j txnJournal) Advance(ctx context.Context, id string, phase txn.State) error {
	return j.store.Update(func(document *state.Document) error {
		for i := range document.Transactions {
			if document.Transactions[i].ID == id {
				document.Transactions[i].State = string(phase)
				return nil
			}
		}
		return nil
	})
}

func (j txnJournal) Finish(ctx context.Context, id string, outcome txn.Outcome) error {
	return j.store.Update(func(document *state.Document) error {
		for i := range document.Transactions {
			if document.Transactions[i].ID == id {
				document.Transactions[i].State = string(outcome.State)
				document.Transactions[i].Result = string(outcome.Result)
				document.Transactions[i].Reason = string(outcome.Reason)
				return nil
			}
		}
		return nil
	})
}

func (j txnJournal) Open(ctx context.Context) ([]txn.OpenRecord, error) {
	document := j.store.Current()
	var records []txn.OpenRecord
	for _, record := range document.Transactions {
		state := txn.State(record.State)
		if !state.Open() {
			continue
		}
		records = append(records, txn.OpenRecord{
			ID: record.ID, Host: record.Host, Stacks: record.Stacks, State: state, Snapshot: record.Snapshot, SelfUpdate: record.SelfUpdate,
		})
	}
	return records, nil
}

type txnSnapshot struct{}

func (txnSnapshot) Estimate(ctx context.Context, plan txn.Plan) (int64, error) { return 0, nil }
func (txnSnapshot) Take(ctx context.Context, plan txn.Plan) (string, error) {
	return "snap-" + plan.ID[len("txn-"):], nil
}
func (txnSnapshot) Validate(ctx context.Context, snapshot string) error            { return nil }
func (txnSnapshot) Restore(ctx context.Context, snapshot string) error             { return nil }
func (txnSnapshot) Discard(ctx context.Context, snapshot string) error             { return nil }
func (txnSnapshot) Retain(ctx context.Context, host, stack string, keep int) error { return nil }

type txnRecreator struct {
	engine *engine.Docker
}

func (r txnRecreator) Replacement(ctx context.Context, plan txn.Plan, container string) (string, error) {
	spec, err := r.engine.Inspect(ctx, container)
	if err != nil {
		return "", err
	}
	for _, target := range plan.Targets {
		if target.Container == container && spec.Config != nil {
			spec.Config.Image = target.Image
		}
	}
	if spec.Config != nil {
		spec.Config.Labels = labelSet(spec.Config.Labels)
		spec.Config.Labels["parachute.transaction"] = plan.ID
		spec.Config.Labels["parachute.snapshot"] = plan.Snapshot
	}
	return r.engine.Create(ctx, spec)
}

func labelSet(labels map[string]string) map[string]string {
	if labels == nil {
		return map[string]string{}
	}
	return labels
}

type txnVerifier struct{}

func (txnVerifier) Verify(ctx context.Context, plan txn.Plan, container string) (txn.Verdict, error) {
	return txn.Verdict{Passed: true}, nil
}
