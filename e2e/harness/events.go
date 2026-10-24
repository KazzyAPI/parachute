package harness

import (
	"bufio"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"time"
)

type EventRecorder struct {
	env    *Env
	events []string
	lock   sync.Mutex
	done   chan struct{}
}

func (e *Env) StartEventRecorder(labelFilter string) *EventRecorder {
	e.T.Helper()
	recorder := &EventRecorder{env: e, done: make(chan struct{})}
	args := []string{"events", "--format", "{{.Type}} {{.Action}} {{.Actor.Attributes.name}}", "--filter", "label=parachute-e2e=" + e.ID}
	if labelFilter != "" {
		args = append(args, "--filter", "label="+labelFilter)
	}
	command := exec.Command("docker", args...)
	output, err := command.StdoutPipe()
	if err != nil {
		e.T.Fatal(err)
	}
	if err := command.Start(); err != nil {
		e.T.Fatal(err)
	}
	go func() {
		reader := bufio.NewScanner(output)
		for reader.Scan() {
			recorder.lock.Lock()
			recorder.events = append(recorder.events, reader.Text())
			recorder.lock.Unlock()
		}
		close(recorder.done)
	}()
	e.T.Cleanup(func() {
		_ = command.Process.Kill()
		_ = command.Wait()
		<-recorder.done
	})
	return recorder
}

func (r *EventRecorder) Events() []string {
	r.lock.Lock()
	defer r.lock.Unlock()
	return append([]string{}, r.events...)
}

func inOrder(got, want []string) bool {
	index := 0
	for _, expected := range want {
		for index < len(got) && !strings.Contains(got[index], expected) {
			index++
		}
		if index >= len(got) {
			return false
		}
		index++
	}
	return true
}

func (r *EventRecorder) AssertOrder(want ...string) {
	r.env.T.Helper()
	if got := r.Events(); !inOrder(got, want) {
		r.env.T.Fatalf("event order: want %q in %v", want, got)
	}
}

func (r *EventRecorder) Wait(timeout time.Duration, want ...string) {
	r.env.T.Helper()
	r.env.Eventually(timeout, fmt.Sprintf("events %q", want), func() bool { return inOrder(r.Events(), want) })
}
