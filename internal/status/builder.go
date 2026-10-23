package status

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/state"
)

const cycleID = "status"

type Builder struct {
	hosts    []pipeline.Host
	discover pipeline.Stage
	redactor Redactor
	armed    bool
}

func NewBuilder(hosts []pipeline.Host, discover pipeline.Stage, redactor Redactor, armed bool) *Builder {
	return &Builder{hosts: hosts, discover: discover, redactor: redactor, armed: armed}
}

func (b *Builder) Build(ctx context.Context, saved state.Document, generated time.Time) (Document, error) {
	cycle := pipeline.NewCycle(cycleID, generated, b.armed, b.hosts)
	if err := b.discover.Run(ctx, cycle); err != nil {
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
	for _, run := range cycle.Hosts {
		host := Host{Name: run.Host.Name, Reachable: run.Reachable, Stacks: []Stack{}}
		for _, candidate := range run.Candidates {
			container := candidate.Container
			host.Stacks = b.place(host.Stacks, container, b.service(container, previous[run.Host.Name+"/"+container.Name]))
		}
		document.Hosts = append(document.Hosts, host)
	}
	return document, nil
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
		service.Hold = &Hold{Reason: previous.Reason, Until: previous.HoldUntil}
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
