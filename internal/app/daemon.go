package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Announcer interface {
	Announce(ctx context.Context, level domain.Level, body string)
	Wait()
}

type ReadyMarker interface {
	Ready() error
	Clear() error
}

type CycleLoop interface {
	Run(ctx context.Context) error
}

type StartupGuard interface {
	Check(ctx context.Context) error
}

type DaemonParts struct {
	Guard          StartupGuard
	Announcer      Announcer
	Marker         ReadyMarker
	Loop           CycleLoop
	Server         Server
	Summary        StartupSummary
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
	if err := d.parts.Guard.Check(ctx); err != nil {
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
	if err := d.serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	d.parts.Logger.Info("Parachute stopped")
	return nil
}

func (d *Daemon) serve(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	served := make(chan error, 1)
	go func() {
		err := d.parts.Server.Serve(ctx)
		if err != nil {
			cancel()
		}
		served <- err
	}()
	loopErr := d.parts.Loop.Run(ctx)
	cancel()
	if serveErr := <-served; serveErr != nil {
		return serveErr
	}
	return loopErr
}

func (d *Daemon) clearMarker() {
	if err := d.parts.Marker.Clear(); err != nil {
		d.parts.Logger.Error("cannot clear readiness marker", "error", err.Error())
	}
}
