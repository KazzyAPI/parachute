package status

import (
	"context"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/state"
)

type ContainerSource interface {
	Containers(ctx context.Context) ([]domain.Container, error)
}

type Selector interface {
	Select(containers []domain.Container) []domain.Container
}

type Builder struct {
	hostName string
	engine   ContainerSource
	selector Selector
	armed    bool
}

func NewBuilder(hostName string, engine ContainerSource, selector Selector, armed bool) *Builder {
	return &Builder{hostName: hostName, engine: engine, selector: selector, armed: armed}
}

func (b *Builder) Build(ctx context.Context, saved state.Document, generated time.Time) (Document, error) {
	containers, err := b.engine.Containers(ctx)
	if err != nil {
		return Document{}, err
	}
	document := Document{Format: Format, Generated: generated.UTC(), Armed: b.armed, Hosts: []Host{}}
	if saved.LastCycle != nil {
		document.LastCycle = &Cycle{ID: saved.LastCycle.ID, Started: saved.LastCycle.Started, Ended: saved.LastCycle.Ended}
	}
	previous := map[string]domain.Result{}
	for _, result := range saved.Results {
		previous[result.Host+"/"+result.Container] = result
	}
	host := Host{Name: b.hostName, Reachable: true, Stacks: []Stack{}}
	for _, container := range b.selector.Select(containers) {
		host.Stacks = b.place(host.Stacks, container, b.service(container, previous[b.hostName+"/"+container.Name]))
	}
	document.Hosts = append(document.Hosts, host)
	return document, nil
}

func (b *Builder) place(stacks []Stack, container domain.Container, service Service) []Stack {
	for index := range stacks {
		if stacks[index].Name == container.Stack() {
			stacks[index].Services = append(stacks[index].Services, service)
			return stacks
		}
	}
	return append(stacks, Stack{Name: container.Stack(), Compose: container.IsCompose(), Services: []Service{service}})
}

func (b *Builder) service(container domain.Container, previous domain.Result) Service {
	service := Service{
		Name: container.Service(), Container: container.Name, Image: container.ImageRef, ImageID: container.ImageID,
		UpdateAvailable: previous.UpdateAvailable(),
	}
	if previous.CandidateDigest != "" {
		digest := previous.CandidateDigest
		service.CandidateDigest = &digest
	}
	if previous.State == domain.StateHeld {
		service.Hold = &Hold{Reason: previous.Reason}
	}
	return service
}
