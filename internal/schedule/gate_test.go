package schedule

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

var errNoRandomness = errors.New("no randomness")

type quietCounter struct{}

func (quietCounter) CycleStarted() {}

func (quietCounter) CycleSkipped() {}

type failingIDs struct{}

func (failingIDs) New(time.Time) (string, error) { return "", errNoRandomness }

func TestGateRunsOneCycleAtATime(t *testing.T) {
	gate := NewGate(fixedIDs{}, &jumpingClock{}, quietCounter{})
	inside := make(chan struct{})
	release := make(chan struct{})
	done := make(chan domain.Report)
	go func() {
		report, _ := gate.Run(context.Background(), func(_ context.Context, id string) domain.Report {
			close(inside)
			<-release
			return domain.Report{CycleID: id}
		})
		done <- report
	}()
	<-inside
	var busy *BusyError
	if _, err := gate.Run(context.Background(), idleRunner{}.Run); !errors.As(err, &busy) || busy.Running != "cyc" {
		t.Errorf("second run returned %v", err)
	}
	close(release)
	if report := <-done; report.CycleID != "cyc" {
		t.Errorf("first run report %+v", report)
	}
	if _, err := gate.Run(context.Background(), idleRunner{}.Run); err != nil {
		t.Errorf("gate not released after the cycle: %v", err)
	}
}

func TestSchedulerStopsWhenNoCycleIDCanBeMade(t *testing.T) {
	clock := &jumpingClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	logger := slog.New(slog.DiscardHandler)
	scheduler := NewScheduler(NewIntervalTrigger(time.Minute), &blockingClock{jumpingClock: clock}, NewGate(failingIDs{}, clock, quietCounter{}), idleRunner{}, logger, time.UTC)
	if err := scheduler.Run(context.Background()); !errors.Is(err, errNoRandomness) {
		t.Fatalf("Run returned %v", err)
	}
}

type blockingClock struct {
	*jumpingClock
	slept bool
}

func (c *blockingClock) Sleep(ctx context.Context, _ time.Duration) error {
	if !c.slept {
		c.slept = true
		return nil
	}
	<-ctx.Done()
	return ctx.Err()
}
