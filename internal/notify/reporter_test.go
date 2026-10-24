package notify

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/logging"
)

type collectingSender struct {
	lock   sync.Mutex
	bodies []string
}

func (c *collectingSender) Send(_ context.Context, _, body string) {
	c.lock.Lock()
	c.bodies = append(c.bodies, body)
	c.lock.Unlock()
}

type noEntries struct{}

func (noEntries) Take(string) []logging.Entry { return nil }

type showAll struct{}

func (showAll) Hidden(domain.Report) (func(domain.Result) bool, error) {
	return func(domain.Result) bool { return false }, nil
}

type neverSleeps struct{}

func (neverSleeps) Sleep(context.Context, time.Duration) error { return errors.New("cancelled") }

func TestRollbackFailureIsSentWhateverTheLevelAndDelay(t *testing.T) {
	sender := &collectingSender{}
	renderer, err := NewRenderer("", true)
	if err != nil {
		t.Fatal(err)
	}
	reporter := NewReporter(config.Notify{URLs: []string{"generic://x"}, Report: true, Delay: time.Minute}, ReporterParts{
		Sender: sender, Views: NewViewBuilder(domain.LevelError, "host"), Renderer: renderer, Sleeper: neverSleeps{},
		Held: showAll{}, Entries: noEntries{}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	report := domain.Report{CycleID: "cyc-1", Hosts: []domain.HostReport{{Name: "local", Reachable: true, Results: []domain.Result{
		{Container: "a", State: domain.StateRollbackFailed, Reason: "restore_failed", Transaction: "txn-1", Snapshot: "snap-1"},
		{Container: "b", State: domain.StateUpdated},
	}}}}
	if err := reporter.Publish(context.Background(), report); err != nil {
		t.Fatal(err)
	}
	reporter.Wait()
	if len(sender.bodies) != 1 || !strings.Contains(sender.bodies[0], "txn-1") || !strings.Contains(sender.bodies[0], "rollback_failed") {
		t.Errorf("sent %v", sender.bodies)
	}
}
