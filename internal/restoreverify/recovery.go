package restoreverify

import (
	"context"
)

type Recoverer struct {
	files FileSystem
	clock Clock
}

func NewRecoverer(files FileSystem, clock Clock) *Recoverer {
	return &Recoverer{files: files, clock: clock}
}

func (r *Recoverer) Recover(ctx context.Context, engines map[string]Engine, dir string) error {
	record, ok, err := ReadRecord(r.files, dir)
	if err != nil || !ok {
		return err
	}
	open := OpenRunID(record)
	if open == "" {
		return nil
	}
	for _, engine := range engines {
		_ = CleanupRuns(ctx, engine, []string{open})
	}
	record.Open = false
	record.Ended = r.clock.Now().UTC()
	for index := range record.Stacks {
		if record.Stacks[index].Result == "" {
			record.Stacks[index].Result = resultFail
			record.Stacks[index].Reason = reasonRestoreFailed
		}
	}
	return WriteRecord(r.files, dir, record)
}

func (r *Recoverer) StartupCleanup(ctx context.Context, engines map[string]Engine, dir string) error {
	locker, err := AcquireLock(r.files, dir)
	if err != nil {
		return nil
	}
	defer func() { _ = locker.Release() }()
	return r.Recover(ctx, engines, dir)
}
