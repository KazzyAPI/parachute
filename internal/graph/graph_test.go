package graph

import (
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/txn"
)

func container(name, stack, service string, labels map[string]string, links []string, networkMode string) domain.Container {
	if labels == nil {
		labels = map[string]string{}
	}
	if stack != "" {
		labels["com.docker.compose.project"] = stack
		labels["com.docker.compose.service"] = service
	}
	return domain.Container{Name: name, Labels: labels, Links: links, NetworkMode: networkMode, State: "running"}
}

func TestDEP_004_label_overrides_link(t *testing.T) {
	index := NewIndex([]domain.Container{
		container("a", "", "", map[string]string{"parachute.depends-on": "cache"}, []string{"/db:/db"}, ""),
		container("db", "", "", nil, nil, ""),
		container("cache", "", "", nil, nil, ""),
	})
	edges := Edges(index, index.ByName["a"])
	if len(edges) != 1 || edges[0].Dependency != "cache" {
		t.Fatalf("edges %v", edges)
	}
}

func TestDEP_008_cycle_detection(t *testing.T) {
	containers := []domain.Container{
		container("a", "", "", map[string]string{"parachute.depends-on": "b"}, nil, ""),
		container("b", "", "", map[string]string{"parachute.depends-on": "a"}, nil, ""),
	}
	index := NewIndex(containers)
	plan := NewBuilder().Plan(index, map[string]txn.Target{
		"a": {Container: "a"},
		"b": {Container: "b"},
	}, "")
	if len(plan.Cycles) != 1 || len(plan.Cycles[0].Containers) != 2 {
		t.Fatalf("cycles %v", plan.Cycles)
	}
}

func TestDEP_005_stop_and_start_order(t *testing.T) {
	edges := []Edge{{Dependent: "web", Dependency: "db", Condition: ConditionStarted}}
	stop := StopOrder([]string{"web", "db"}, edges)
	if stop[0] != "web" || stop[1] != "db" {
		t.Fatalf("stop %v", stop)
	}
	start := StartSequence([]string{"web", "db"}, edges)
	if start[0] != "db" || start[1] != "web" {
		t.Fatalf("start %v", start)
	}
}

func TestUPD_013_rolling_groups(t *testing.T) {
	edges := []Edge{{Dependent: "b", Dependency: "a", Condition: ConditionStarted}}
	groups := RollingGroups([]string{"a", "b", "c"}, edges)
	if len(groups) != 2 {
		t.Fatalf("groups %v", groups)
	}
}
