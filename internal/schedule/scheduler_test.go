package schedule

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type jumpingClock struct {
	lock   sync.Mutex
	now    time.Time
	sleeps []time.Duration
	jump   time.Duration
}

func (c *jumpingClock) Now() time.Time {
	c.lock.Lock()
	defer c.lock.Unlock()
	return c.now
}

func (c *jumpingClock) Sleep(_ context.Context, duration time.Duration) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.sleeps = append(c.sleeps, duration)
	if len(c.sleeps) > 4 {
		return context.Canceled
	}
	c.now = c.now.Add(max(duration, 0))
	if len(c.sleeps) == 2 {
		c.now = c.now.Add(c.jump)
	}
	return nil
}

type fixedIDs struct{}

func (fixedIDs) New(time.Time) (string, error) { return "cyc", nil }

type idleRunner struct{}

func (idleRunner) Run(context.Context, string) domain.Report { return domain.Report{} }

func TestSchedulerSkipsSlotsMissedDuringAClockJump(t *testing.T) {
	clock := &jumpingClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), jump: time.Hour}
	logger := slog.New(slog.DiscardHandler)
	scheduler := NewScheduler(NewIntervalTrigger(time.Minute), clock, NewGate(fixedIDs{}, clock, logger), idleRunner{}, logger, time.UTC)
	if err := scheduler.Run(context.Background()); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run returned %v", err)
	}
	for index, duration := range clock.sleeps[1:] {
		if duration <= 0 {
			t.Errorf("sleep %d after the jump is %s; missed slots are replayed", index+1, duration)
		}
	}
}
