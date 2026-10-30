package graph

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/txn"
)

type callLog struct {
	calls []string
}

type recordingSelfRunner struct {
	log *callLog
}

func (r *recordingSelfRunner) Handover(context.Context, string, *pipeline.Candidate) (HandoverTicket, error) {
	r.log.calls = append(r.log.calls, "handover")
	return HandoverTicket{}, nil
}

func (r *recordingSelfRunner) Finish(context.Context, HandoverTicket) error {
	r.log.calls = append(r.log.calls, "finish")
	return nil
}

type recordingSink struct {
	log *callLog
}

func (s *recordingSink) Publish(context.Context, domain.Report) error {
	s.log.calls = append(s.log.calls, "report")
	return nil
}

func TestSelfRunnerHandoverAfterGroupsFinishAfterReport(t *testing.T) {
	log := &callLog{}
	runner := &recordingSelfRunner{log: log}
	h := newHarness(fakeSnapshots{}, fakeRecreator{}, fakeVerifier{txn.Verdict{Passed: true}})
	h.executor.selfRunner = runner
	stage := NewStage(map[string]*Executor{"local": h.executor}, "")
	report := pipeline.NewReportStage(fakeClock{}, slog.New(slog.NewTextHandler(io.Discard, nil)), &recordingSink{log: log})
	pipe := pipeline.New(stage, report, NewFinishStage(runner))
	cycle := pipeline.NewCycle("cyc-1", fakeClock{}.Now(), true, []pipeline.Host{{Name: "local"}})
	host := cycle.Hosts[0]
	host.Containers = h.containers
	host.Candidates = h.candidates
	if err := pipe.Run(context.Background(), cycle); err != nil {
		t.Fatal(err)
	}
	want := []string{"handover", "report", "finish"}
	if !slices.Equal(log.calls, want) {
		t.Fatalf("calls %v want %v", log.calls, want)
	}
}
