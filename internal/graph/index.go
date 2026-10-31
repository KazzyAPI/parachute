package graph

import (
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Index struct {
	All        []domain.Container
	ByName     map[string]domain.Container
	ByStack    map[string][]domain.Container
	ByStackSvc map[string]map[string][]domain.Container
}

func NewIndex(containers []domain.Container) Index {
	index := Index{
		All:        containers,
		ByName:     map[string]domain.Container{},
		ByStack:    map[string][]domain.Container{},
		ByStackSvc: map[string]map[string][]domain.Container{},
	}
	for _, container := range containers {
		index.ByName[container.Name] = container
		stack := container.Stack()
		index.ByStack[stack] = append(index.ByStack[stack], container)
		if index.ByStackSvc[stack] == nil {
			index.ByStackSvc[stack] = map[string][]domain.Container{}
		}
		service := container.Service()
		index.ByStackSvc[stack][service] = append(index.ByStackSvc[stack][service], container)
	}
	return index
}

func (i Index) Resolve(name string) (domain.Container, bool) {
	container, ok := i.ByName[name]
	return container, ok
}

func (i Index) ResolveContainerRef(name string) (domain.Container, bool) {
	if container, ok := i.Resolve(name); ok {
		return container, true
	}
	for _, container := range i.All {
		if strings.HasPrefix(container.ID, name) || container.ID == name {
			return container, true
		}
	}
	return domain.Container{}, false
}
