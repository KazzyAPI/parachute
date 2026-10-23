package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type failingSource struct {
	err   error
	calls int
}

func (s *failingSource) Containers(context.Context) ([]domain.Container, error) {
	s.calls++
	return nil, s.err
}

func (s *failingSource) Image(context.Context, string) (Image, bool, error) {
	s.calls++
	return Image{}, false, s.err
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time { return c.now }

func (c *fakeClock) Sleep(_ context.Context, duration time.Duration) error {
	c.now = c.now.Add(duration)
	return nil
}

func newTestRetrying(source Source) *Retrying {
	policy := RetryPolicy{FirstDelay: time.Second, MaxDelay: 30 * time.Second, Window: 5 * time.Minute, AttemptTimeout: time.Second}
	return NewRetrying(source, &fakeClock{now: time.Unix(0, 0)}, policy, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func TestTransportErrorsRetryUntilTheWindowThenTheHostIsDown(t *testing.T) {
	source := &failingSource{err: io.ErrUnexpectedEOF}
	retrying := newTestRetrying(source)
	var unreachable *UnreachableError
	if _, err := retrying.Containers(context.Background()); !errors.As(err, &unreachable) {
		t.Fatalf("error %v, want unreachable", err)
	}
	if source.calls < 5 {
		t.Errorf("%d attempts, want retries", source.calls)
	}
	source.calls = 0
	if _, _, err := retrying.Image(context.Background(), "x"); !errors.As(err, &unreachable) || source.calls != 1 {
		t.Errorf("down host: %d attempts, error %v; want one attempt", source.calls, err)
	}
}

func TestEngineErrorsAreNotRetriedAndMarkTheHostUp(t *testing.T) {
	source := &failingSource{err: errors.New("engine answered 500")}
	retrying := newTestRetrying(source)
	retrying.MarkUnreachable()
	if _, err := retrying.Containers(context.Background()); err == nil || source.calls != 1 {
		t.Fatalf("%d attempts, error %v", source.calls, err)
	}
	source.err, source.calls = io.EOF, 0
	if _, err := retrying.Containers(context.Background()); err == nil || source.calls < 2 {
		t.Errorf("host back up should retry: %d attempts", source.calls)
	}
}
