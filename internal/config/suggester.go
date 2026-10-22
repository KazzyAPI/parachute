package config

import (
	"fmt"
	"slices"
)

type Suggester struct {
	maxDistance int
}

func NewSuggester(maxDistance int) *Suggester {
	return &Suggester{maxDistance: maxDistance}
}

func (s *Suggester) Unknown(name string, known []string) error {
	slices.Sort(known)
	best, bestDistance := "", s.maxDistance+1
	for _, candidate := range known {
		if distance := s.distance(name, candidate); distance < bestDistance {
			best, bestDistance = candidate, distance
		}
	}
	if best == "" {
		return fmt.Errorf("unknown setting %s", name)
	}
	return fmt.Errorf("unknown setting %s, did you mean %s?", name, best)
}

func (s *Suggester) distance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}
