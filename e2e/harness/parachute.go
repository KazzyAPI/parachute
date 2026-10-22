package harness

import (
	"bytes"
	"context"
	"errors"
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
	if len(args) > 0 && (args[0] == "healthcheck" || args[0] == "status") {
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
	return append(command, args...)
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := p.build(ctx)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	started := time.Now()
	err := cmd.Run()
	result := &Result{env: p.env, Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(started)}
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		p.env.T.Fatalf("parachute did not exit within %s\nstderr:\n%s", timeout, result.Stderr)
	case errors.As(err, &exitErr):
		result.Code = exitErr.ExitCode()
	case err != nil:
		p.env.T.Fatalf("run parachute: %v", err)
	}
	p.env.T.Logf("parachute %v exited %d after %s\nstderr:\n%s", cmd.Args[1:], result.Code, result.Duration, result.Stderr)
	return result
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
