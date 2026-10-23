package harness

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
	"syscall"
	"time"
)

type Parachute struct {
	env      *Env
	args     []string
	vars     map[string]string
	unset    []string
	unscoped bool
}

func (e *Env) Parachute(args ...string) *Parachute {
	return &Parachute{env: e, args: args, vars: map[string]string{}}
}

func (p *Parachute) Unscoped() *Parachute {
	p.unscoped = true
	return p
}

func (p *Parachute) Setenv(name, value string) *Parachute {
	p.vars[name] = value
	return p
}

func (p *Parachute) Unsetenv(name string) *Parachute {
	p.unset = append(p.unset, name)
	return p
}

func (p *Parachute) commandArgs() []string {
	var command []string
	args := append([]string{}, p.args...)
	if len(args) > 0 && isCommand(args[0]) {
		command, args = args[:1], args[1:]
	}
	has := func(flag string) bool {
		return slices.ContainsFunc(args, func(arg string) bool { return arg == flag || strings.HasPrefix(arg, flag+"=") })
	}
	if !has("--state-dir") {
		args = append(args, "--state-dir", p.env.StateDir)
	}
	if !has("--log-format") {
		args = append(args, "--log-format", "json")
	}
	if !p.unscoped && !has("--scope") {
		args = append(args, "--scope", p.env.Scope())
	}
	if !has("--delay-days") && !has("--profile") && !p.setsAny("PARACHUTE_DELAY_DAYS", "PARACHUTE_PROFILE") {
		args = append(args, "--delay-days", "0")
	}
	return append(command, args...)
}

func (p *Parachute) setsAny(names ...string) bool {
	return slices.ContainsFunc(names, func(name string) bool { _, ok := p.vars[name]; return ok })
}

func (p *Parachute) environment() []string {
	vars := map[string]string{
		"PATH":          os.Getenv("PATH"),
		"HOME":          p.env.Dir,
		"DOCKER_HOST":   suite.dockerHost,
		"DOCKER_CONFIG": p.env.ConfigDir,
	}
	for name, value := range p.vars {
		vars[name] = value
	}
	for _, name := range p.unset {
		delete(vars, name)
	}
	environment := make([]string, 0, len(vars))
	for name, value := range vars {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func (p *Parachute) build(ctx context.Context) *exec.Cmd {
	cmd := exec.CommandContext(ctx, suite.binary, p.commandArgs()...)
	cmd.Env = p.environment()
	return cmd
}

func (p *Parachute) Run(timeout time.Duration) *Result {
	p.env.T.Helper()
	return p.env.runCommand(timeout, p.environment(), suite.binary, p.commandArgs()...)
}

func (p *Parachute) Start() *Process {
	p.env.T.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	cmd := p.build(ctx)
	process := newProcess(p.env, cmd, cancel)
	if err := cmd.Start(); err != nil {
		cancel()
		p.env.T.Fatalf("start parachute: %v", err)
	}
	go process.wait()
	p.env.T.Cleanup(func() {
		if process.running() {
			_ = cmd.Process.Signal(syscall.SIGKILL)
		}
		<-process.done
		cancel()
		p.env.T.Logf("daemon stderr:\n%s", process.stderr.String())
	})
	return process
}
