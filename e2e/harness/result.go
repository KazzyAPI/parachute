package harness

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

type Log struct {
	Raw      string
	Fields   map[string]any
	Received time.Time
}

func ParseLog(text string, received time.Time) Log {
	entry := Log{Raw: text, Received: received}
	var fields map[string]any
	if json.Unmarshal([]byte(text), &fields) == nil {
		entry.Fields = fields
	}
	return entry
}

func (l Log) JSON() bool { return l.Fields != nil }

func (l Log) Get(key string) string {
	value, ok := l.Fields[key]
	if !ok || value == nil {
		return ""
	}
	if text, isText := value.(string); isText {
		return text
	}
	return fmt.Sprint(value)
}

func (l Log) Msg() string { return l.Get("msg") }

func (l Log) Level() string { return l.Get("level") }

func (l Log) Time() time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, l.Get("time"))
	if err != nil {
		return time.Time{}
	}
	return parsed
}

func isCommand(arg string) bool {
	return slices.Contains([]string{"healthcheck", "status", "restore-verify"}, arg)
}

func (e *Env) runCommand(timeout time.Duration, environ []string, name string, args ...string) *Result {
	e.T.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = environ
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	started := time.Now()
	err := cmd.Run()
	result := &Result{env: e, Stdout: stdout.String(), Stderr: stderr.String(), Duration: time.Since(started)}
	var exitErr *exec.ExitError
	switch {
	case ctx.Err() != nil:
		e.T.Fatalf("%s did not exit within %s\nstderr:\n%s", filepath.Base(name), timeout, result.Stderr)
	case errors.As(err, &exitErr):
		result.Code = exitErr.ExitCode()
	case err != nil:
		e.T.Fatalf("run %s: %v", filepath.Base(name), err)
	}
	e.T.Logf("%s %v exited %d after %s\nstderr:\n%s", filepath.Base(name), args, result.Code, result.Duration, result.Stderr)
	return result
}

type Result struct {
	env      *Env
	Stdout   string
	Stderr   string
	Code     int
	Duration time.Duration
}

func (r *Result) Logs() []Log {
	var logs []Log
	for _, line := range strings.Split(strings.TrimSpace(r.Stderr), "\n") {
		if line != "" {
			logs = append(logs, ParseLog(line, time.Time{}))
		}
	}
	return logs
}

func (r *Result) StdoutLines() []string {
	trimmed := strings.TrimSpace(r.Stdout)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func (r *Result) Porcelain() []map[string]any {
	r.env.T.Helper()
	var objects []map[string]any
	for _, line := range r.StdoutLines() {
		var object map[string]any
		if err := json.Unmarshal([]byte(line), &object); err != nil {
			r.env.T.Fatalf("porcelain line is not JSON: %q: %v", line, err)
		}
		objects = append(objects, object)
	}
	return objects
}

func (r *Result) Entry(container string) map[string]any {
	for _, object := range r.Porcelain() {
		if object["type"] == "entry" && object["container"] == container {
			return object
		}
	}
	return nil
}

func (r *Result) Candidates() []string {
	var names []string
	for _, object := range r.Porcelain() {
		if object["type"] == "entry" {
			names = append(names, fmt.Sprint(object["container"]))
		}
	}
	return names
}

func (r *Result) Summary() map[string]any {
	objects := r.Porcelain()
	if len(objects) == 0 {
		return nil
	}
	return objects[len(objects)-1]
}
