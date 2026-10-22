package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type StartupEngine interface {
	Verify(ctx context.Context) error
	Containers(ctx context.Context) ([]domain.Container, error)
}

type Announcer interface {
	Announce(ctx context.Context, level domain.Level, body string)
	Wait()
}

type ReadyMarker interface {
	Ready() error
	Clear() error
}

type OnceRunner interface {
	Run(ctx context.Context, id string) domain.Report
}

type LoopRunner interface {
	Run(ctx context.Context) error
}

type IDGenerator interface {
	New(started time.Time) (string, error)
}

type LabelReader interface {
	Bool(container domain.Container, name string) (bool, bool)
}

type Hostnamer interface {
	Hostname() (string, error)
}

type DaemonParts struct {
	Engine         StartupEngine
	Endpoint       string
	Announcer      Announcer
	Marker         ReadyMarker
	Once           OnceRunner
	Loop           LoopRunner
	IDs            IDGenerator
	Clock          Clock
	Hostname       Hostnamer
	Labels         LabelReader
	Summary        StartupSummary
	RunOnce        bool
	StartupMessage bool
	Logger         *slog.Logger
}

type Daemon struct {
	parts DaemonParts
}

func NewDaemon(parts DaemonParts) *Daemon {
	return &Daemon{parts: parts}
}

func (d *Daemon) Run(ctx context.Context) error {
	if err := d.parts.Engine.Verify(ctx); err != nil {
		return unreachableError(fmt.Errorf("no managed host is reachable: %s: %w", d.parts.Endpoint, err))
	}
	if err := d.refuseArmedLabel(ctx); err != nil {
		return err
	}
	d.parts.Logger.Info("Parachute started", d.parts.Summary.Fields()...)
	if d.parts.StartupMessage {
		d.parts.Announcer.Announce(ctx, domain.LevelInfo, d.parts.Summary.Text())
	}
	if err := d.parts.Marker.Ready(); err != nil {
		return err
	}
	defer d.clearMarker()
	defer d.parts.Announcer.Wait()
	if d.parts.RunOnce {
		return d.runOnce(ctx)
	}
	if err := d.parts.Loop.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	d.parts.Logger.Info("Parachute stopped")
	return nil
}

func (d *Daemon) runOnce(ctx context.Context) error {
	id, err := d.parts.IDs.New(d.parts.Clock.Now())
	if err != nil {
		return err
	}
	report := d.parts.Once.Run(ctx, id)
	if code := reportExitCode(report); code != ExitSuccess {
		return &ExitError{Code: code, Err: fmt.Errorf("cycle %s finished with problems", id)}
	}
	return nil
}

func (d *Daemon) refuseArmedLabel(ctx context.Context) error {
	hostname, err := d.parts.Hostname.Hostname()
	if err != nil {
		return fmt.Errorf("cannot determine own hostname: %w", err)
	}
	if len(hostname) < 12 {
		return nil
	}
	containers, err := d.parts.Engine.Containers(ctx)
	if err != nil {
		return unreachableError(err)
	}
	for _, container := range containers {
		if armed, _ := d.parts.Labels.Bool(container, "armed"); armed && strings.HasPrefix(container.ID, hostname) {
			return configurationError(errors.New("updates are not implemented in this milestone (parachute.armed=true on own container)"))
		}
	}
	return nil
}

func (d *Daemon) clearMarker() {
	if err := d.parts.Marker.Clear(); err != nil {
		d.parts.Logger.Error("cannot clear readiness marker", "error", err.Error())
	}
}
