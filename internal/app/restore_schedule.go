package app

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/restoreverify"
	"github.com/KazzyAPI/parachute/internal/schedule"
)

type restoreRunner func(ctx context.Context) restoreverify.Outcome

type restoreMetrics interface {
	RestoreVerified(host, stack string, at time.Time)
}

type scheduledRestoreVerify struct {
	inner   CycleLoop
	trigger schedule.Trigger
	clock   schedule.Clock
	gate    *schedule.Gate
	run     restoreRunner
	report  Announcer
	metrics restoreMetrics
	logger  *slog.Logger
}

func (s scheduledRestoreVerify) Run(ctx context.Context) error {
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	done := make(chan struct{})
	go func() {
		defer close(done)
		s.loop(ctx)
	}()
	err := s.inner.Run(ctx)
	stop()
	<-done
	return err
}

func (s scheduledRestoreVerify) loop(ctx context.Context) {
	next := s.trigger.First(s.clock.Now())
	for {
		if err := s.clock.Sleep(ctx, next.Sub(s.clock.Now())); err != nil {
			return
		}
		s.once(ctx)
		next = s.trigger.Next(next)
		for now := s.clock.Now(); !next.After(now); {
			next = s.trigger.Next(next)
		}
	}
}

func (s scheduledRestoreVerify) once(ctx context.Context) {
	release, err := s.gate.Hold("restore-verify")
	if err != nil {
		s.logger.Info("scheduled restore verification skipped, a cycle is running", "error", err.Error())
		return
	}
	defer release()
	outcome := s.run(ctx)
	if outcome.Err != nil && outcome.Document.ID == "" {
		s.logger.Info("scheduled restore verification did not run", "error", outcome.Err.Error())
		return
	}
	announceRestoreVerify(ctx, s.report, outcome.Document)
	for _, stack := range outcome.Document.Stacks {
		if stack.Result == "pass" {
			s.metrics.RestoreVerified(stack.Host, stack.Stack, outcome.Document.Ended)
		}
	}
}

func announceRestoreVerify(ctx context.Context, report Announcer, document restoreverify.Document) {
	if document.ID == "" {
		return
	}
	level := domain.LevelInfo
	var lines []string
	for _, stack := range document.Stacks {
		line := fmt.Sprintf("%s/%s: %s", stack.Host, stack.Stack, stack.Result)
		if stack.Reason != "" {
			line += " (" + stack.Reason + ")"
		}
		if stack.Result != "pass" {
			level = domain.LevelWarn
		}
		if !stack.LastPass.IsZero() && !document.Ended.IsZero() {
			line += fmt.Sprintf(", last passing restore %s ago", document.Ended.Sub(stack.LastPass).Round(time.Second))
		}
		lines = append(lines, line)
	}
	report.Announce(ctx, level, "Restore verification "+document.ID+"\n"+strings.Join(lines, "\n"))
}

func (c *Container) restoreLock() (func(), error) {
	locker, err := restoreverify.AcquireLock(c.deps.System, c.cfg.StateDir)
	if err != nil {
		return nil, err
	}
	return func() { _ = locker.Release() }, nil
}
