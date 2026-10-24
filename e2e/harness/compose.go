package harness

import (
	"os"
	"path/filepath"
	"strings"
)

type ComposeProject struct {
	env  *Env
	dir  string
	name string
}

func (e *Env) ComposeProject(name string, composeYAML string) *ComposeProject {
	e.T.Helper()
	dir := filepath.Join(e.Dir, "compose-"+name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		e.T.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(composeYAML), 0o644); err != nil {
		e.T.Fatal(err)
	}
	project := &ComposeProject{env: e, dir: dir, name: e.Name(name)}
	project.writeLabelOverride()
	e.T.Cleanup(func() {
		if out, err := e.DockerMayFail(project.args("down", "-v")...); err != nil {
			e.T.Logf("cleanup: compose down: %v %s", err, out)
		}
	})
	return project
}

func (p *ComposeProject) writeLabelOverride() {
	p.env.T.Helper()
	services := strings.Fields(p.env.Docker("compose", "-p", p.name, "-f", filepath.Join(p.dir, "compose.yaml"), "config", "--services"))
	var override strings.Builder
	override.WriteString("services:\n")
	for _, service := range services {
		override.WriteString("  " + service + ":\n    labels:\n      parachute-e2e: \"" + p.env.ID + "\"\n")
	}
	if err := os.WriteFile(filepath.Join(p.dir, "override.yaml"), []byte(override.String()), 0o644); err != nil {
		p.env.T.Fatal(err)
	}
}

func (p *ComposeProject) args(command ...string) []string {
	files := []string{"compose", "-p", p.name, "-f", filepath.Join(p.dir, "compose.yaml"), "-f", filepath.Join(p.dir, "override.yaml")}
	return append(files, command...)
}

func (p *ComposeProject) Up(args ...string) {
	p.env.T.Helper()
	p.env.Docker(p.args(append([]string{"up", "-d"}, args...)...)...)
}

func (p *ComposeProject) Down() {
	p.env.T.Helper()
	p.env.Docker(p.args("down", "-v")...)
}

func (p *ComposeProject) Dir() string { return p.dir }
