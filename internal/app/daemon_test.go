package app

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type passingGuard struct{}

func (passingGuard) Check(context.Context) error { return nil }

type quietMarker struct{}

func (quietMarker) Ready() error { return nil }

func (quietMarker) Clear() error { return nil }

type recordingLoop struct {
	ctx   context.Context
	block bool
}

func (l *recordingLoop) Run(ctx context.Context) error {
	l.ctx = ctx
	if l.block {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

type checkingAnnouncer struct {
	loop            *recordingLoop
	cancelledAtWait bool
}

func (a *checkingAnnouncer) Announce(context.Context, domain.Level, string) {}

func (a *checkingAnnouncer) Wait() { a.cancelledAtWait = a.loop.ctx.Err() != nil }

type failingServer struct{ err error }

func (s failingServer) Serve(context.Context) error { return s.err }

func newTestDaemon(loop *recordingLoop, announcer *checkingAnnouncer, server Server) *Daemon {
	return NewDaemon(DaemonParts{
		Guard: passingGuard{}, Announcer: announcer, Marker: quietMarker{}, Loop: loop, Server: server,
		Logger: slog.New(slog.DiscardHandler),
	})
}

func TestServerErrorStopsTheLoop(t *testing.T) {
	failure := errors.New("listen failed")
	loop := &recordingLoop{block: true}
	err := newTestDaemon(loop, &checkingAnnouncer{loop: loop}, failingServer{err: failure}).Run(context.Background())
	if !errors.Is(err, failure) {
		t.Fatalf("Run returned %v", err)
	}
}

func TestNotificationsDrainBeforeTheCycleContextIsCancelled(t *testing.T) {
	loop := &recordingLoop{}
	announcer := &checkingAnnouncer{loop: loop}
	if err := newTestDaemon(loop, announcer, idleServer{}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if announcer.cancelledAtWait {
		t.Error("cycle context was cancelled before notifications drained")
	}
}
