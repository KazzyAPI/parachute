package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/handover"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type handoverEngine interface {
	Containers(ctx context.Context) ([]domain.Container, error)
	Inspect(ctx context.Context, id string) (engine.Spec, error)
	Remove(ctx context.Context, id string, anonymousVolumes, force bool) error
	Image(ctx context.Context, reference string) (engine.Image, bool, error)
}

type handoverSleep interface {
	Sleep(ctx context.Context, duration time.Duration) error
}

type realHandoverSleep struct{}

func (realHandoverSleep) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type handoverReady interface {
	Ready() error
}

type handoverPublisher interface {
	Publish(ctx context.Context, report domain.Report) error
}

type StartupHandover struct {
	stateDir string
	files    handover.FileSystem
	engine   handoverEngine
	self     engine.Self
	host     string
	own      domain.Container
	admit    handover.Admission
	takeover bool
	journal  *state.Journal
	reporter handoverPublisher
	rollback *RollbackFinisher
	ready    handoverReady
	sleeper  handoverSleep
	logger   *slog.Logger
}

func (c *Container) startupHandover(host connectedHost) *StartupHandover {
	return &StartupHandover{
		stateDir: c.cfg.StateDir,
		files:    c.deps.System,
		engine:   host.docker,
		self:     c.self(),
		host:     host.spec.Name,
		sleeper:  realHandoverSleep{},
		logger:   c.logger,
	}
}

func (s *StartupHandover) bind(journal *state.Journal, reporter handoverPublisher, finisher *RollbackFinisher, ready handoverReady) {
	s.journal = journal
	s.reporter = reporter
	s.rollback = finisher
	s.ready = ready
}

func (s *StartupHandover) BeforeStore(ctx context.Context) error {
	marker, hasMarker, err := handover.Read(s.files, s.stateDir)
	if err != nil {
		return configurationError(handover.InvalidMarkerError(err))
	}
	if !s.self.Contained() || s.engine == nil {
		if !hasMarker {
			return nil
		}
		return configurationError(errors.New("handover.json"))
	}
	containers, err := s.engine.Containers(ctx)
	if err != nil {
		if !hasMarker {
			return nil
		}
		return unreachableError(err)
	}
	if !s.self.Mark([]engine.SelfHost{{Local: true, Containers: containers}}) {
		if !hasMarker {
			return nil
		}
		return configurationError(errors.New("handover.json"))
	}
	for _, container := range containers {
		if container.Own {
			s.own = container
			break
		}
	}
	if !hasMarker {
		return nil
	}
	if s.own.ID == "" {
		return configurationError(errors.New("handover.json"))
	}
	digests, err := s.ownDigests(ctx)
	if err != nil {
		return unreachableError(err)
	}
	admit, err := handover.Classify(marker, true, s.own.ID, s.own.Labels["parachute.transaction"], digests)
	if err != nil {
		return configurationError(err)
	}
	s.admit = admit
	if admit.Role != handover.RoleNew {
		return nil
	}
	s.logger.Info("waiting for handover", "transaction", marker.Transaction)
	if s.ready != nil {
		if err := s.ready.Ready(); err != nil {
			return err
		}
	}
	for {
		if err := s.waitOnce(ctx, marker); err != nil {
			return err
		}
		marker, hasMarker, err = handover.Read(s.files, s.stateDir)
		if err != nil {
			return configurationError(handover.InvalidMarkerError(err))
		}
		if !hasMarker {
			return nil
		}
		admit, err = handover.Classify(marker, true, s.own.ID, s.own.Labels["parachute.transaction"], digests)
		if err != nil {
			return configurationError(err)
		}
		s.admit = admit
		if admit.Role != handover.RoleNew {
			return nil
		}
	}
}

func (s *StartupHandover) waitOnce(ctx context.Context, marker handover.Marker) error {
	containers, err := s.engine.Containers(ctx)
	if err != nil {
		return unreachableError(err)
	}
	old, found := containerByID(containers, marker.Container)
	if !handover.OldContainerActive(found, old.State) {
		s.takeover = true
		return nil
	}
	return s.sleeper.Sleep(ctx, handover.PollInterval)
}

func (s *StartupHandover) AfterStore(ctx context.Context, store *state.Store) error {
	if s.own.ID == "" {
		return nil
	}
	document := store.Current()
	if slices.Contains(document.SupersededInstances, s.own.ID) {
		return configurationError(errors.New("superseded Parachute instance"))
	}
	marker, hasMarker, err := handover.Read(s.files, s.stateDir)
	if err != nil {
		return configurationError(handover.InvalidMarkerError(err))
	}
	if !hasMarker {
		return nil
	}
	if !s.admit.HasMarker {
		digests, err := s.ownDigests(ctx)
		if err != nil {
			return unreachableError(err)
		}
		admit, err := handover.Classify(marker, true, s.own.ID, s.own.Labels["parachute.transaction"], digests)
		if err != nil {
			return configurationError(err)
		}
		s.admit = admit
	}
	record, ok := transactionRecord(document, marker.Transaction)
	if !ok {
		return configurationError(fmt.Errorf("handover marker names unknown transaction %s", marker.Transaction))
	}
	switch {
	case s.takeover:
		return s.finishTakeover(ctx, marker, record)
	case s.admit.Role == handover.RoleOld:
		return s.finishOldRestart(ctx, marker, record)
	default:
		return nil
	}
}

func (s *StartupHandover) finishTakeover(ctx context.Context, marker handover.Marker, record state.TransactionRecord) error {
	state := txn.State(record.State)
	if state == txn.StateCommitted {
		if err := handover.Remove(s.files, s.stateDir); err != nil {
			return err
		}
		return s.removeOldContainer(ctx, marker, record)
	}
	if !state.Open() || state == txn.StateSnapshot {
		return configurationError(fmt.Errorf("handover marker with terminal transaction state %s", record.State))
	}
	outcome := txn.Outcome{
		State: txn.StateRollbackFail, Result: domain.StateRollbackFailed, Reason: "interrupted", Superseded: marker.Container,
	}
	if err := s.journal.Finish(ctx, marker.Transaction, outcome); err != nil {
		return err
	}
	if err := handover.Remove(s.files, s.stateDir); err != nil {
		return err
	}
	result := domain.NewResult(s.host, s.own)
	result = result.Conclude(domain.StateRollbackFailed, "interrupted", "")
	result.Transaction = marker.Transaction
	report := domain.Report{CycleID: "handover", Hosts: []domain.HostReport{{Name: s.host, Results: []domain.Result{result}}}}
	if err := s.reporter.Publish(ctx, report); err != nil {
		return err
	}
	return nil
}

func (s *StartupHandover) finishOldRestart(ctx context.Context, marker handover.Marker, record state.TransactionRecord) error {
	state := txn.State(record.State)
	if state == txn.StateCommitted {
		if err := handover.Remove(s.files, s.stateDir); err != nil {
			return err
		}
		return configurationError(errors.New("superseded Parachute instance"))
	}
	if state != txn.StateHandover && state != txn.StateRollingBack {
		return nil
	}
	open := txn.OpenRecord{
		ID: record.ID, Host: record.Host, Stacks: slices.Clone(record.Stacks), State: state,
		Snapshot: record.Snapshot, SelfUpdate: record.SelfUpdate, OwnContainer: record.OwnContainer, Originals: slices.Clone(record.Originals),
	}
	_, results, err := RecoverInterruptedSelfHandover(ctx, s.rollback, s.journal, open)
	if err != nil {
		return err
	}
	if err := handover.Remove(s.files, s.stateDir); err != nil {
		return err
	}
	if len(results) == 0 {
		return nil
	}
	report := domain.Report{CycleID: "recovery", Hosts: []domain.HostReport{{Name: s.host, Results: results}}}
	return s.reporter.Publish(ctx, report)
}

func (s *StartupHandover) removeOldContainer(ctx context.Context, marker handover.Marker, record state.TransactionRecord) error {
	containers, err := s.engine.Containers(ctx)
	if err != nil {
		return unreachableError(err)
	}
	old, found := containerByID(containers, marker.Container)
	if !found {
		return nil
	}
	if record.OwnContainer == "" {
		return nil
	}
	want := handover.RenamedName(record.OwnContainer, marker.Transaction)
	if old.ID != marker.Container || old.Name != want {
		return nil
	}
	return s.engine.Remove(ctx, old.ID, false, true)
}

func (s *StartupHandover) ownDigests(ctx context.Context) ([]string, error) {
	ref := s.own.ImageRef
	if ref == "" {
		ref = s.own.ImageID
	}
	image, ok, err := s.engine.Image(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}
	return image.Digests, nil
}

func containerByID(containers []domain.Container, id string) (domain.Container, bool) {
	for _, container := range containers {
		if container.ID == id {
			return container, true
		}
	}
	return domain.Container{}, false
}

func transactionRecord(document state.Document, id string) (state.TransactionRecord, bool) {
	for _, record := range document.Transactions {
		if record.ID == id {
			return record, true
		}
	}
	return state.TransactionRecord{}, false
}

func (x *RollbackFinisher) RollbackSelfHandover(ctx context.Context, record txn.OpenRecord) (txn.Outcome, []*pipeline.Candidate, error) {
	plan := txn.Plan{ID: record.ID, Host: record.Host, Stacks: record.Stacks, SelfUpdate: true, Snapshot: record.Snapshot}
	if _, err := x.removeReplacements(ctx, record.ID); err != nil {
		return txn.Outcome{}, nil, err
	}
	if _, _, err := x.restoreOriginals(ctx, plan, record.Originals); err != nil {
		return txn.Outcome{}, nil, err
	}
	var candidates []*pipeline.Candidate
	for _, original := range record.Originals {
		if !original.Updated {
			continue
		}
		container := domain.Container{ID: original.ID, Name: record.OwnContainer}
		result := domain.NewResult(record.Host, container)
		result = result.Conclude(domain.StateRolledBack, "interrupted", "")
		result.Transaction = record.ID
		candidates = append(candidates, &pipeline.Candidate{Container: container, Result: result})
	}
	return txn.Outcome{State: txn.StateRolledBack, Result: domain.StateRolledBack, Reason: "interrupted"}, candidates, nil
}
