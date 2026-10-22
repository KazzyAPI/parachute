package engine

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/moby/moby/client"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Source interface {
	Containers(ctx context.Context) ([]domain.Container, error)
	Image(ctx context.Context, reference string) (Image, bool, error)
}

type Clock interface {
	Now() time.Time
	Sleep(ctx context.Context, duration time.Duration) error
}

type RetryPolicy struct {
	FirstDelay     time.Duration
	MaxDelay       time.Duration
	Window         time.Duration
	AttemptTimeout time.Duration
}

type Retrying struct {
	source Source
	clock  Clock
	policy RetryPolicy
	logger *slog.Logger
}

func NewRetrying(source Source, clock Clock, policy RetryPolicy, logger *slog.Logger) *Retrying {
	return &Retrying{source: source, clock: clock, policy: policy, logger: logger}
}

func (r *Retrying) Containers(ctx context.Context) ([]domain.Container, error) {
	var containers []domain.Container
	err := r.do(ctx, "list containers", func(attempt context.Context) error {
		var err error
		containers, err = r.source.Containers(attempt)
		return err
	})
	return containers, err
}

func (r *Retrying) Image(ctx context.Context, reference string) (Image, bool, error) {
	var image Image
	var found bool
	err := r.do(ctx, "inspect image", func(attempt context.Context) error {
		var err error
		image, found, err = r.source.Image(attempt, reference)
		return err
	})
	return image, found, err
}

func (r *Retrying) do(ctx context.Context, operation string, call func(context.Context) error) error {
	deadline := r.clock.Now().Add(r.policy.Window)
	delay := r.policy.FirstDelay
	for {
		attempt, cancel := context.WithTimeout(ctx, r.policy.AttemptTimeout)
		err := call(attempt)
		cancel()
		if err == nil || !r.retryable(ctx, err) {
			return err
		}
		if !r.clock.Now().Add(delay).Before(deadline) {
			return &UnreachableError{Err: err}
		}
		r.logger.Warn("Docker request failed, retrying", "operation", operation, "retry_in", delay.String(), "error", err.Error())
		if sleepErr := r.clock.Sleep(ctx, delay); sleepErr != nil {
			return sleepErr
		}
		delay = min(delay*2, r.policy.MaxDelay)
	}
}

func (r *Retrying) retryable(ctx context.Context, err error) bool {
	if ctx.Err() != nil {
		return false
	}
	var netErr net.Error
	return client.IsErrConnectionFailed(err) ||
		errors.As(err, &netErr) ||
		errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, context.DeadlineExceeded)
}

type UnreachableError struct {
	Err error
}

func (e *UnreachableError) Error() string { return "host unreachable: " + e.Err.Error() }

func (e *UnreachableError) Unwrap() error { return e.Err }
