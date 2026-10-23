package harness

import (
	"os"
	"path/filepath"
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
	return &ComposeProject{env: e, dir: dir, name: e.Name(name)}
}

func (p *ComposeProject) Up(args ...string) {
	p.env.T.Helper()
	command := []string{"compose", "-p", p.name, "-f", filepath.Join(p.dir, "compose.yaml"), "up", "-d"}
	p.env.Docker(append(command, args...)...)
}

func (p *ComposeProject) Down() {
	p.env.T.Helper()
	p.env.Docker("compose", "-p", p.name, "-f", filepath.Join(p.dir, "compose.yaml"), "down", "-v")
}

func (p *ComposeProject) Dir() string { return p.dir }
