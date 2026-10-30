package graph

import (
	"cmp"
	"slices"
	"strings"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type Member struct {
	Container domain.Container
	Update    bool
	Recreate  bool
}

type Group struct {
	Stacks  []string
	Members []Member
	Targets []txn.Target
}

type CycleFailure struct {
	Containers []string
}

type Plan struct {
	Groups  []Group
	Cycles  []CycleFailure
	Skipped map[string]CycleFailure
}

type Builder struct{}

func NewBuilder() *Builder { return &Builder{} }

func (b *Builder) Plan(index Index, targets map[string]txn.Target, own string) Plan {
	edges := AllEdges(index)
	affected := b.affected(index, targets, edges)
	if len(affected) == 0 {
		return Plan{}
	}
	plan := Plan{}
	for {
		cycleNames := cycles(affected, edges)
		if len(cycleNames) == 0 {
			break
		}
		plan.Cycles = append(plan.Cycles, CycleFailure{Containers: cycleNames})
		for _, name := range cycleNames {
			delete(affected, name)
			delete(targets, name)
		}
	}
	if len(affected) == 0 {
		return plan
	}
	plan.Groups = b.groups(index, affected, targets, edges, own)
	return plan
}

func (b *Builder) affected(index Index, targets map[string]txn.Target, edges []Edge) map[string]Member {
	affected := map[string]Member{}
	for name := range targets {
		container, ok := index.ByName[name]
		if !ok {
			continue
		}
		b.markReplica(index, affected, container, true)
	}
	dependents := map[string][]string{}
	for _, edge := range edges {
		dependents[edge.Dependency] = append(dependents[edge.Dependency], edge.Dependent)
	}
	var queue []string
	for name := range affected {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		for _, dependent := range dependents[name] {
			if _, ok := affected[dependent]; ok {
				continue
			}
			container, ok := index.ByName[dependent]
			if !ok {
				continue
			}
			recreate := mustRecreate(index, container, name)
			affected[dependent] = Member{Container: container, Recreate: recreate}
			queue = append(queue, dependent)
		}
	}
	for name, member := range affected {
		if member.Update {
			continue
		}
		for _, edge := range edges {
			if edge.Dependent != name {
				continue
			}
			if _, updating := targets[edge.Dependency]; updating {
				member.Recreate = member.Recreate || mustRecreate(index, member.Container, edge.Dependency)
			}
		}
		affected[name] = member
	}
	return affected
}

func (b *Builder) markReplica(index Index, affected map[string]Member, container domain.Container, update bool) {
	stack := container.Stack()
	service := container.Service()
	for _, replica := range index.ByStackSvc[stack][service] {
		member := Member{Container: replica, Update: update}
		if existing, ok := affected[replica.Name]; ok {
			member.Recreate = existing.Recreate
			if existing.Update {
				member.Update = true
			}
		}
		affected[replica.Name] = member
	}
}

func mustRecreate(index Index, container domain.Container, dependency string) bool {
	return networkDependsOn(index, container, dependency) || linkDependsOn(index, container, dependency)
}

func linkDependsOn(index Index, container domain.Container, dependency string) bool {
	if strings.TrimSpace(container.Labels[labelDependsOn]) != "" {
		return false
	}
	return slices.ContainsFunc(container.Links, func(link string) bool {
		target, ok := linkTarget(link, index)
		return ok && target.Name == dependency
	})
}

func networkDependsOn(index Index, container domain.Container, dependency string) bool {
	target, ok := networkContainer(container.NetworkMode, index)
	return ok && target.Name == dependency
}

func (b *Builder) groups(index Index, affected map[string]Member, targets map[string]txn.Target, edges []Edge, own string) []Group {
	parent := map[string]string{}
	find := func(name string) string {
		for parent[name] != name && parent[name] != "" {
			name = parent[name]
		}
		return name
	}
	union := func(left, right string) {
		left, right = find(left), find(right)
		if left == right {
			return
		}
		parent[right] = left
	}
	for name := range affected {
		if parent[name] == "" {
			parent[name] = name
		}
	}
	for _, edge := range edges {
		if _, ok := affected[edge.Dependent]; !ok {
			continue
		}
		if _, ok := affected[edge.Dependency]; !ok {
			continue
		}
		if edge.Dependent == own || edge.Dependency == own {
			continue
		}
		union(index.ByName[edge.Dependent].Stack(), index.ByName[edge.Dependency].Stack())
	}
	buckets := map[string][]Member{}
	for _, member := range affected {
		if member.Container.Own || member.Container.Name == own {
			continue
		}
		root := find(member.Container.Stack())
		buckets[root] = append(buckets[root], member)
	}
	var groups []Group
	for _, members := range buckets {
		stacks := map[string]bool{}
		targetList := []txn.Target{}
		for _, member := range members {
			stacks[member.Container.Stack()] = true
			if member.Update {
				if target, ok := targets[member.Container.Name]; ok {
					targetList = append(targetList, target)
				}
			}
		}
		stackNames := mapsKeys(stacks)
		slices.Sort(stackNames)
		slices.SortFunc(members, func(left, right Member) int {
			return cmp.Compare(left.Container.Name, right.Container.Name)
		})
		slices.SortFunc(targetList, func(left, right txn.Target) int {
			return cmp.Compare(left.Container, right.Container)
		})
		groups = append(groups, Group{Stacks: stackNames, Members: members, Targets: targetList})
	}
	slices.SortFunc(groups, func(left, right Group) int {
		if len(left.Stacks) == 0 || len(right.Stacks) == 0 {
			return cmp.Compare(len(left.Stacks), len(right.Stacks))
		}
		return cmp.Compare(left.Stacks[0], right.Stacks[0])
	})
	return groups
}

func mapsKeys[K comparable, V any](values map[K]V) []K {
	keys := make([]K, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func cycles(affected map[string]Member, edges []Edge) []string {
	graph := map[string][]string{}
	inDegree := map[string]int{}
	for name := range affected {
		inDegree[name] = 0
		graph[name] = nil
	}
	for _, edge := range edges {
		if _, ok := affected[edge.Dependent]; !ok {
			continue
		}
		if _, ok := affected[edge.Dependency]; !ok {
			continue
		}
		graph[edge.Dependency] = append(graph[edge.Dependency], edge.Dependent)
		inDegree[edge.Dependent]++
	}
	var queue []string
	for name, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, name)
		}
	}
	visited := 0
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		visited++
		for _, dependent := range graph[name] {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}
	if visited == len(affected) {
		return nil
	}
	remaining := map[string]bool{}
	for name := range affected {
		if inDegree[name] > 0 {
			remaining[name] = true
		}
	}
	names := mapsKeys(remaining)
	slices.Sort(names)
	return names
}
