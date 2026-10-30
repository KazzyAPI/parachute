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

func TestDEP_007_network_mode_by_id_marks_dependent_for_recreation(t *testing.T) {
	app := container("app", "", "", nil, nil, "")
	app.ID = "abc123def456"
	sidecar := container("sidecar", "", "", nil, nil, "container:abc123def456")
	index := NewIndex([]domain.Container{app, sidecar})
	plan := NewBuilder().Plan(index, map[string]txn.Target{"app": {Container: "app"}}, "")
	if len(plan.Groups) != 1 {
		t.Fatalf("groups %v", plan.Groups)
	}
	for _, member := range plan.Groups[0].Members {
		if member.Container.Name == "sidecar" && member.Recreate {
			return
		}
	}
	t.Fatalf("sidecar not marked for recreation: %v", plan.Groups[0].Members)
}

func TestDEP_004_compose_depends_on_label_carries_condition(t *testing.T) {
	index := NewIndex([]domain.Container{
		container("shop-db-1", "shop", "db", nil, nil, ""),
		container("shop-job-1", "shop", "job", nil, nil, ""),
		container("shop-app-1", "shop", "app", map[string]string{"com.docker.compose.depends_on": "db:service_healthy:true,job:service_completed_successfully:false"}, nil, ""),
	})
	want := map[string]Condition{"shop-db-1": ConditionHealthy, "shop-job-1": ConditionExitOK}
	edges := Edges(index, index.ByName["shop-app-1"])
	if len(edges) != 2 {
		t.Fatalf("edges %v", edges)
	}
	for _, edge := range edges {
		if edge.Condition != want[edge.Dependency] {
			t.Errorf("edge %v", edge)
		}
	}
}

func TestDEP_005_linked_dependent_is_recreated(t *testing.T) {
	db := container("db", "", "", nil, nil, "")
	web := container("web", "", "", nil, []string{"/db:/web/db"}, "")
	index := NewIndex([]domain.Container{db, web})
	plan := NewBuilder().Plan(index, map[string]txn.Target{"db": {Container: "db"}}, "")
	for _, member := range plan.Groups[0].Members {
		if member.Container.Name == "web" && member.Recreate {
			return
		}
	}
	t.Fatalf("web not marked for recreation: %v", plan.Groups[0].Members)
}
