package e2e

import (
	"os"
	"testing"

	"github.com/KazzyAPI/parachute/e2e/harness"
)

func TestDEP_001_compose_project_forms_a_stack(t *testing.T) {
	env := harness.NewEnv(t)
	if _, err := env.DockerMayFail("compose", "version"); err != nil {
		t.Skip("docker compose is not installed")
	}
	image := fixture(env)
	project := env.Name("shop")
	compose := env.WriteFile("compose/compose.yaml", `services:
  web:
    image: `+image+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
  db:
    image: `+image+`
    labels: {parachute.scope: "`+env.Scope()+`", parachute-e2e: "`+env.ID+`"}
`, 0o644)
	env.Docker("compose", "-p", project, "-f", compose, "up", "-d")
	if err := os.Remove(compose); err != nil {
		t.Fatal(err)
	}
	result := runOnce(env)
	services := map[string]bool{}
	for _, entry := range result.Porcelain() {
		if entry["type"] == "entry" && entry["stack"] == project {
			services[entry["service"].(string)] = true
		}
	}
	if !services["web"] || !services["db"] || len(services) != 2 {
		t.Errorf("stack %s services %v, want web and db", project, services)
	}
}

func TestDEP_002_plain_container_is_a_stack_of_one(t *testing.T) {
	env := harness.NewEnv(t)
	solo := env.RunContainer("solo", fixture(env), harness.ContainerOptions{})
	entry := assertState(t, runOnce(env), solo, "fresh", "")
	if entry["stack"] != solo || entry["service"] != solo {
		t.Errorf("stack %v service %v, want %s", entry["stack"], entry["service"], solo)
	}
}
