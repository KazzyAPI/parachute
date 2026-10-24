package graph

import (
	"cmp"
	"slices"
)

func StopOrder(names []string, edges []Edge) []string {
	return topo(names, edges, true)
}

func StartSequence(names []string, edges []Edge) []string {
	return topo(names, edges, false)
}

func topo(names []string, edges []Edge, dependentsFirst bool) []string {
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	graph := map[string][]string{}
	inDegree := map[string]int{}
	for _, name := range names {
		inDegree[name] = 0
		graph[name] = nil
	}
	for _, edge := range edges {
		if !set[edge.Dependent] || !set[edge.Dependency] {
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
	slices.Sort(queue)
	var order []string
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		order = append(order, name)
		for _, next := range graph[name] {
			inDegree[next]--
			if inDegree[next] == 0 {
				queue = append(queue, next)
				slices.Sort(queue)
			}
		}
	}
	if len(order) != len(names) {
		sorted := slices.Clone(names)
		slices.Sort(sorted)
		return sorted
	}
	if dependentsFirst {
		slices.Reverse(order)
	}
	return order
}

func MemberNames(members []Member) []string {
	names := make([]string, len(members))
	for i, member := range members {
		names[i] = member.Container.Name
	}
	slices.Sort(names)
	return names
}

func FilterEdges(edges []Edge, names []string) []Edge {
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	var filtered []Edge
	for _, edge := range edges {
		if set[edge.Dependent] && set[edge.Dependency] {
			filtered = append(filtered, edge)
		}
	}
	slices.SortFunc(filtered, func(left, right Edge) int {
		return cmp.Or(cmp.Compare(left.Dependency, right.Dependency), cmp.Compare(left.Dependent, right.Dependent))
	})
	return filtered
}
