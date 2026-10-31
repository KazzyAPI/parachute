package status

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/txn"
)

const cycleID = "status"

type Builder struct {
	hosts     []pipeline.Host
	inventory pipeline.Stage
	redactor  Redactor
	armed     bool
	lock      sync.Mutex
}

func NewBuilder(hosts []pipeline.Host, inventory pipeline.Stage, redactor Redactor, armed bool) *Builder {
	return &Builder{hosts: hosts, inventory: inventory, redactor: redactor, armed: armed}
}

func (b *Builder) Build(ctx context.Context, saved state.Document, generated time.Time) (Document, error) {
	cycle, err := b.list(ctx, generated)
	if err != nil {
		return Document{}, fmt.Errorf("list containers: %w", err)
	}
	if len(cycle.Hosts) > 0 && len(cycle.ReachableHosts()) == 0 {
		return Document{}, errors.New("no managed host is reachable")
	}
	document := Document{Format: Format, Generated: generated.UTC(), Armed: b.armed, Hosts: []Host{}}
	if saved.LastCycle != nil {
		document.LastCycle = &Cycle{ID: saved.LastCycle.ID, Started: saved.LastCycle.Started, Ended: saved.LastCycle.Ended}
	}
	previous := map[string]domain.Result{}
	for _, result := range saved.Results {
		previous[result.Host+"/"+result.Container] = result
	}
	transactions := map[string]state.TransactionRecord{}
	for _, record := range saved.Transactions {
		transactions[record.ID] = record
	}
	latest := map[string]state.TransactionRecord{}
	for _, link := range saved.ServiceTransactions {
		if record, known := transactions[link.Transaction]; known {
			latest[link.Host+"/"+link.Container] = record
		}
	}
	for _, run := range cycle.Hosts {
		host := Host{Name: run.Host.Name, Reachable: run.Reachable, Stacks: []Stack{}}
		for _, candidate := range run.Candidates {
			container := candidate.Container
			service := b.service(container, previous[run.Host.Name+"/"+container.Name])
			if record, known := latest[run.Host.Name+"/"+container.Name]; known {
				service.LastTransaction = transaction(record)
			}
			host.Stacks = b.place(host.Stacks, container, service)
		}
		host.Stacks = b.freeze(host.Stacks, run, saved.ActiveFreezes())
		b.backups(host.Stacks, run.Host.Name, saved.Snapshots)
		document.Hosts = append(document.Hosts, host)
	}
	return document, nil
}

func (b *Builder) list(ctx context.Context, generated time.Time) (*pipeline.Cycle, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	cycle := pipeline.NewCycle(cycleID, generated, b.armed, b.hosts)
	return cycle, b.inventory.Run(ctx, cycle)
}

func (b *Builder) place(stacks []Stack, container domain.Container, service Service) []Stack {
	name := b.redactor.Redact(container.Stack())
	for index := range stacks {
		if stacks[index].Name == name {
			stacks[index].Services = append(stacks[index].Services, service)
			return stacks
		}
	}
	return append(stacks, Stack{Name: name, Compose: container.IsCompose(), Frozen: []Freeze{}, Services: []Service{service}})
}

func (b *Builder) service(container domain.Container, previous domain.Result) Service {
	service := Service{
		Name: b.redactor.Redact(container.Service()), Container: b.redactor.Redact(container.Name),
		Image: b.redactor.Redact(container.ImageRef), ImageID: container.ImageID,
		UpdateAvailable: previous.UpdateAvailable(), Drift: b.redacted(previous.Drift),
	}
	if previous.CandidateDigest != "" {
		digest := previous.CandidateDigest
		service.CandidateDigest = &digest
	}
	if previous.State == domain.StateHeld {
		service.Hold = &Hold{Reason: b.redactor.Redact(previous.Reason), Until: previous.HoldUntil}
	}
	return service
}

func (b *Builder) redacted(text *string) *string {
	if text == nil {
		return nil
	}
	redacted := b.redactor.Redact(*text)
	return &redacted
}

func transaction(record state.TransactionRecord) *Transaction {
	last := &Transaction{ID: record.ID, State: record.State, Time: record.Time}
	if txn.State(record.State).Terminal() {
		last.Result = optional(record.Result)
		last.Reason = optional(record.Reason)
	}
	return last
}

func optional(text string) *string {
	if text == "" {
		return nil
	}
	return &text
}

func (b *Builder) backups(stacks []Stack, host string, snapshots []state.SnapshotRecord) {
	for _, record := range snapshots {
		if record.Host != host || record.ID == state.NoSnapshot {
			continue
		}
		name := b.redactor.Redact(record.Stack)
		for index := range stacks {
			last := stacks[index].LastBackup
			if stacks[index].Name == name && (last == nil || !record.Time.Before(last.Time)) {
				stacks[index].LastBackup = &Backup{Snapshot: record.ID, Time: record.Time, Valid: true}
			}
		}
	}
}

func (b *Builder) freeze(stacks []Stack, run *pipeline.HostRun, freezes []state.Freeze) []Stack {
	for _, freeze := range freezes {
		entry := Freeze{Transaction: freeze.Transaction, Snapshot: freeze.Snapshot, Containers: []string{}}
		for _, container := range run.Containers {
			if run.Reachable && freeze.Covers(run.Host.Name, container) {
				entry.Containers = append(entry.Containers, b.redactor.Redact(container.Name))
			}
		}
		for _, key := range freeze.Keys {
			if key.Host != run.Host.Name {
				continue
			}
			stacks = b.frozen(stacks, key, run.Containers, entry)
		}
	}
	return stacks
}

func (b *Builder) frozen(stacks []Stack, key state.FreezeKey, containers []domain.Container, entry Freeze) []Stack {
	name := key.Stack
	for _, container := range containers {
		if key.Container == container.Name {
			name = container.Stack()
		}
	}
	name = b.redactor.Redact(name)
	index := slices.IndexFunc(stacks, func(stack Stack) bool { return stack.Name == name })
	if index < 0 {
		stacks = append(stacks, Stack{Name: name, Frozen: []Freeze{}, Services: []Service{}})
		index = len(stacks) - 1
	}
	if !slices.ContainsFunc(stacks[index].Frozen, func(listed Freeze) bool { return listed.Transaction == entry.Transaction }) {
		stacks[index].Frozen = append(stacks[index].Frozen, entry)
	}
	return stacks
}
