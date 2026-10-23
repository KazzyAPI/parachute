package schedule

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type failingIDs struct{}

func (failingIDs) New(time.Time) (string, error) { return "", errors.New("no randomness") }

func TestGateRunsOneCycleAtATime(t *testing.T) {
	gate := NewGate(fixedIDs{}, &jumpingClock{}, slog.New(slog.DiscardHandler))
	inside := make(chan struct{})
	release := make(chan struct{})
	done := make(chan domain.Report)
	go func() {
		report, _, _ := gate.Run(context.Background(), func(_ context.Context, id string) domain.Report {
			close(inside)
			<-release
			return domain.Report{CycleID: id}
		})
		done <- report
	}()
	<-inside
	if _, running, started := gate.Run(context.Background(), idleRunner{}.Run); started || running != "cyc" {
		t.Errorf("second run started=%t running=%q", started, running)
	}
	close(release)
	if report := <-done; report.CycleID != "cyc" {
		t.Errorf("first run report %+v", report)
	}
	if _, _, started := gate.Run(context.Background(), idleRunner{}.Run); !started {
		t.Error("gate not released after the cycle")
	}
}

func TestGateDoesNotStartWithoutAnID(t *testing.T) {
	gate := NewGate(failingIDs{}, &jumpingClock{}, slog.New(slog.DiscardHandler))
	if _, running, started := gate.Run(context.Background(), idleRunner{}.Run); started || running != "" {
		t.Errorf("started=%t running=%q", started, running)
	}
}
