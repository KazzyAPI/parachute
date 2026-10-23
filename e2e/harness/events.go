package harness

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"sync"
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

func (r *EventRecorder) AssertOrder(want ...string) {
	got := r.Events()
	index := 0
	for _, expected := range want {
		for index < len(got) && !strings.Contains(got[index], expected) {
			index++
		}
		if index >= len(got) {
			r.env.T.Fatalf("event order: missing %q in %v", expected, got)
		}
		index++
	}
}

func (r *EventRecorder) Wait(ctx context.Context) {
	select {
	case <-r.done:
	case <-ctx.Done():
	}
}
