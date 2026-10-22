package harness

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type lines struct {
	lock    sync.Mutex
	entries []Log
}

func (l *lines) add(text string) {
	l.lock.Lock()
	defer l.lock.Unlock()
	l.entries = append(l.entries, ParseLog(text, time.Now()))
}

func (l *lines) snapshot() []Log {
	l.lock.Lock()
	defer l.lock.Unlock()
	return append([]Log{}, l.entries...)
}

func (l *lines) String() string {
	var raw []string
	for _, entry := range l.snapshot() {
		raw = append(raw, entry.Raw)
	}
	return strings.Join(raw, "\n")
}

type Process struct {
	env    *Env
	cmd    *exec.Cmd
	cancel context.CancelFunc
	stdout *lines
	stderr *lines
	done   chan struct{}
	code   int
	pipes  sync.WaitGroup
}

func newProcess(env *Env, cmd *exec.Cmd, cancel context.CancelFunc) *Process {
	process := &Process{env: env, cmd: cmd, cancel: cancel, stdout: &lines{}, stderr: &lines{}, done: make(chan struct{})}
	stdout, stdoutErr := cmd.StdoutPipe()
	stderr, stderrErr := cmd.StderrPipe()
	if err := errors.Join(stdoutErr, stderrErr); err != nil {
		env.T.Fatal(err)
	}
	process.pipes.Go(func() { process.collect(stdout, process.stdout) })
	process.pipes.Go(func() { process.collect(stderr, process.stderr) })
	return process
}

func (p *Process) collect(reader io.Reader, into *lines) {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)
	for scanner.Scan() {
		into.add(scanner.Text())
	}
}

func (p *Process) wait() {
	p.pipes.Wait()
	err := p.cmd.Wait()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		p.code = exitErr.ExitCode()
	}
	close(p.done)
}

func (p *Process) running() bool {
	select {
	case <-p.done:
		return false
	default:
		return true
	}
}

func (p *Process) Logs() []Log { return p.stderr.snapshot() }

func (p *Process) Stdout() []Log { return p.stdout.snapshot() }

func (p *Process) WaitForLog(timeout time.Duration, what string, match func(Log) bool) Log {
	p.env.T.Helper()
	return p.WaitForLogs(timeout, what, 1, match)[0]
}

func (p *Process) WaitForLogs(timeout time.Duration, what string, count int, match func(Log) bool) []Log {
	p.env.T.Helper()
	var found []Log
	p.env.Eventually(timeout, what, func() bool {
		found = found[:0]
		for _, entry := range p.Logs() {
			if match(entry) {
				found = append(found, entry)
			}
		}
		return len(found) >= count
	})
	return found
}

func (p *Process) Signal(signal os.Signal) {
	p.env.T.Helper()
	if err := p.cmd.Process.Signal(signal); err != nil {
		p.env.T.Fatalf("signal parachute: %v", err)
	}
}

func (p *Process) Wait(timeout time.Duration) int {
	p.env.T.Helper()
	select {
	case <-p.done:
		return p.code
	case <-time.After(timeout):
		p.env.T.Fatalf("parachute did not exit within %s", timeout)
		return -1
	}
}

func (p *Process) Running() bool { return p.running() }
