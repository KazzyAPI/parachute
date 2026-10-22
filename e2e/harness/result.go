package harness

import (
	"encoding/json"
	"fmt"
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
