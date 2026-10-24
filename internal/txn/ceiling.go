package txn

import (
	"context"
	"sync"
	"time"
)

type Ceiling struct {
	limit  time.Duration
	clock  func() time.Time
	mu     sync.Mutex
	start  time.Time
	paused time.Duration
	pause  time.Time
	active bool
}

type ceilingKey struct{}

func WithCeiling(ctx context.Context, ceiling *Ceiling) context.Context {
	return context.WithValue(ctx, ceilingKey{}, ceiling)
}

func CeilingFrom(ctx context.Context) *Ceiling {
	ceiling, _ := ctx.Value(ceilingKey{}).(*Ceiling)
	return ceiling
}

func (c *Ceiling) Remaining() time.Duration {
	return c.limit - c.Elapsed()
}

func NewCeiling(limit time.Duration, clock func() time.Time) *Ceiling {
	return &Ceiling{limit: limit, clock: clock}
}

func (c *Ceiling) Begin() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.start = c.clock()
	c.paused = 0
	c.pause = time.Time{}
	c.active = true
}

func (c *Ceiling) Pause() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active || !c.pause.IsZero() {
		return
	}
	c.pause = c.clock()
}

func (c *Ceiling) Resume() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active || c.pause.IsZero() {
		return
	}
	c.paused += c.clock().Sub(c.pause)
	c.pause = time.Time{}
}

func (c *Ceiling) Elapsed() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.active {
		return 0
	}
	elapsed := c.clock().Sub(c.start) - c.paused
	if !c.pause.IsZero() {
		elapsed -= c.clock().Sub(c.pause)
	}
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func (c *Ceiling) Expired() bool {
	if c.limit <= 0 {
		return false
	}
	return c.Elapsed() >= c.limit
}
