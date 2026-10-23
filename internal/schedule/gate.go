package schedule

import (
	"context"
	"log/slog"
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

type Gate struct {
	ids     IDGenerator
	clock   WallClock
	logger  *slog.Logger
	lock    sync.Mutex
	running string
}

func NewGate(ids IDGenerator, clock WallClock, logger *slog.Logger) *Gate {
	return &Gate{ids: ids, clock: clock, logger: logger}
}

func (g *Gate) Run(ctx context.Context, run func(ctx context.Context, id string) domain.Report) (report domain.Report, running string, started bool) {
	id, running, err := g.claim()
	if err != nil {
		g.logger.Error("cannot start cycle", "error", err.Error())
	}
	if id == "" {
		return domain.Report{}, running, false
	}
	defer g.release()
	return run(ctx, id), "", true
}

func (g *Gate) claim() (id, running string, err error) {
	g.lock.Lock()
	defer g.lock.Unlock()
	if g.running != "" {
		return "", g.running, nil
	}
	id, err = g.ids.New(g.clock.Now())
	if err != nil {
		return "", "", err
	}
	g.running = id
	return id, "", nil
}

func (g *Gate) release() {
	g.lock.Lock()
	defer g.lock.Unlock()
	g.running = ""
}
