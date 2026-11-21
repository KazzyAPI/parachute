package schedule

import (
	"context"
	"sync"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type IDGenerator interface {
	New(started time.Time) (string, error)
}

type WallClock interface {
	Now() time.Time
}

type Counter interface {
	CycleStarted()
	CycleSkipped()
}

type BusyError struct {
	Running string
}

func (e *BusyError) Error() string { return "cycle " + e.Running + " is still running" }

type Gate struct {
	ids     IDGenerator
	clock   WallClock
	counter Counter
	lock    sync.Mutex
	running string
	exclude func() (func(), error)
}

func (g *Gate) SetExclusion(acquire func() (func(), error)) { g.exclude = acquire }

func (g *Gate) Hold(name string) (func(), error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.running != "" {
		return nil, &BusyError{Running: g.running}
	}
	g.running = name
	return g.release, nil
}

func NewGate(ids IDGenerator, clock WallClock, counter Counter) *Gate {
	return &Gate{ids: ids, clock: clock, counter: counter}
}

func (g *Gate) Run(ctx context.Context, run func(ctx context.Context, id string) domain.Report) (domain.Report, error) {
	id, err := g.claim()
	if err != nil {
		return domain.Report{}, err
	}
	defer g.release()
	if g.exclude != nil {
		unlock, err := g.exclude()
		if err != nil {
			g.counter.CycleSkipped()
			return domain.Report{}, &BusyError{Running: "restore-verify"}
		}
		defer unlock()
	}
	return run(ctx, id), nil
}

func (g *Gate) claim() (string, error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.running != "" {
		g.counter.CycleSkipped()
		return "", &BusyError{Running: g.running}
	}
	id, err := g.ids.New(g.clock.Now())
	if err != nil {
		return "", err
	}
	g.running = id
	g.counter.CycleStarted()
	return id, nil
}

func (g *Gate) release() {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.running = ""
}
