package graph

import (
	"cmp"
	"slices"
)

func RollingGroups(names []string, edges []Edge) [][]string {
	set := map[string]bool{}
	for _, name := range names {
		set[name] = true
	}
	adjacent := map[string][]string{}
	for _, name := range names {
		adjacent[name] = nil
	}
	for _, edge := range edges {
		if !set[edge.Dependent] || !set[edge.Dependency] {
			continue
		}
		adjacent[edge.Dependent] = appendUnique(adjacent[edge.Dependent], edge.Dependency)
		adjacent[edge.Dependency] = appendUnique(adjacent[edge.Dependency], edge.Dependent)
	}
	visited := map[string]bool{}
	var groups [][]string
	sorted := append([]string(nil), names...)
	slices.Sort(sorted)
	for _, name := range sorted {
		if visited[name] {
			continue
		}
		component := []string{}
		var stack []string
		stack = append(stack, name)
		visited[name] = true
		for len(stack) > 0 {
			current := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			component = append(component, current)
			for _, neighbor := range adjacent[current] {
				if visited[neighbor] {
					continue
				}
				visited[neighbor] = true
				stack = append(stack, neighbor)
			}
		}
		slices.Sort(component)
		groups = append(groups, component)
	}
	slices.SortFunc(groups, func(left, right []string) int {
		if len(left) == 0 || len(right) == 0 {
			return cmp.Compare(len(left), len(right))
		}
		return cmp.Compare(left[0], right[0])
	})
	return groups
}

func appendUnique(values []string, value string) []string {
	if slices.Contains(values, value) {
		return values
	}
	return append(values, value)
}
