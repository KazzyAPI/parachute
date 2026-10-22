package clock

import (
	"context"
	"time"
)

type System struct{}

func NewSystem() *System { return &System{} }

func (System) Now() time.Time { return time.Now() }

func (System) Sleep(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
