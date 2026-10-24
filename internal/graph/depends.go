package graph

import (
	"encoding/json"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
)

const (
	labelDependsOn        = "parachute.depends-on"
	composeDependsOnLabel = "com.docker.compose.depends_on"
)

type Condition int

const (
	ConditionStarted Condition = 0
	ConditionHealthy Condition = 1
	ConditionExitOK  Condition = 2
)

type Edge struct {
	Dependent  string
	Dependency string
	Condition  Condition
}

func Edges(index Index, container domain.Container) []Edge {
	var edges []Edge
	label := strings.TrimSpace(container.Labels[labelDependsOn])
	if label != "" {
		for _, name := range strings.Split(label, ",") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if target, ok := index.Resolve(name); ok {
				edges = append(edges, Edge{Dependent: container.Name, Dependency: target.Name, Condition: ConditionStarted})
			}
		}
	} else {
		for _, link := range container.Links {
			if target, ok := linkTarget(link, index); ok {
				edges = append(edges, Edge{Dependent: container.Name, Dependency: target.Name, Condition: ConditionStarted})
			}
		}
		edges = append(edges, composeDependsOn(container, index)...)
	}
	if target, ok := networkContainer(container.NetworkMode, index); ok {
		edges = append(edges, Edge{Dependent: container.Name, Dependency: target.Name, Condition: ConditionStarted})
	}
	return edges
}

func AllEdges(index Index) []Edge {
	var edges []Edge
	seen := map[string]bool{}
	for _, container := range index.All {
		for _, edge := range Edges(index, container) {
			key := edge.Dependent + "->" + edge.Dependency
			if seen[key] {
				continue
			}
			seen[key] = true
			edges = append(edges, edge)
		}
	}
	return edges
}

func linkTarget(link string, index Index) (domain.Container, bool) {
	link = strings.TrimPrefix(link, "/")
	name, _, _ := strings.Cut(link, ":")
	name, _, _ = strings.Cut(name, "/")
	return index.Resolve(strings.TrimSpace(name))
}

func networkContainer(mode string, index Index) (domain.Container, bool) {
	const prefix = "container:"
	if !strings.HasPrefix(mode, prefix) {
		return domain.Container{}, false
	}
	return index.Resolve(strings.TrimPrefix(mode, prefix))
}

func composeDependsOn(container domain.Container, index Index) []Edge {
	raw := strings.TrimSpace(container.Labels[composeDependsOnLabel])
	if raw == "" || !container.IsCompose() {
		return nil
	}
	stack := container.Stack()
	services := index.ByStackSvc[stack]
	if services == nil {
		return nil
	}
	var edges []Edge
	if strings.HasPrefix(raw, "{") {
		var entries map[string]map[string]any
		if err := json.Unmarshal([]byte(raw), &entries); err != nil {
			return nil
		}
		for service, options := range entries {
			targets := services[service]
			if len(targets) == 0 {
				continue
			}
			condition := ConditionStarted
			if options != nil {
				switch options["condition"] {
				case "service_healthy":
					condition = ConditionHealthy
				case "service_completed_successfully":
					condition = ConditionExitOK
				}
			}
			for _, target := range targets {
				edges = append(edges, Edge{Dependent: container.Name, Dependency: target.Name, Condition: condition})
			}
		}
		return edges
	}
	for _, service := range strings.Split(raw, ",") {
		service = strings.TrimSpace(service)
		if service == "" {
			continue
		}
		for _, target := range services[service] {
			edges = append(edges, Edge{Dependent: container.Name, Dependency: target.Name, Condition: ConditionStarted})
		}
	}
	return edges
}
