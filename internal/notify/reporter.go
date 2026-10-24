package notify

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/logging"
)

type Sender interface {
	Send(ctx context.Context, title, body string)
}

type Sleeper interface {
	Sleep(ctx context.Context, duration time.Duration) error
}

type LogEntries interface {
	Take(cycleID string) []logging.Entry
}

type HeldFilter interface {
	Hidden(report domain.Report) (func(domain.Result) bool, error)
}

type ReporterParts struct {
	Sender   Sender
	Views    *ViewBuilder
	Renderer *Renderer
	Sleeper  Sleeper
	Held     HeldFilter
	Entries  LogEntries
	Logger   *slog.Logger
}

type Reporter struct {
	settings config.Notify
	parts    ReporterParts
	title    string
	pending  sync.WaitGroup
}

func NewReporter(settings config.Notify, parts ReporterParts) *Reporter {
	title := "Parachute on " + settings.Hostname
	if settings.TitleTag != "" {
		title = "[" + settings.TitleTag + "] " + title
	}
	if settings.SkipTitle {
		title = ""
	}
	return &Reporter{settings: settings, parts: parts, title: title}
}

func (r *Reporter) Publish(ctx context.Context, report domain.Report) error {
	var data any = r.parts.Entries.Take(report.CycleID)
	if len(r.settings.URLs) == 0 {
		return nil
	}
	r.announceFailures(ctx, report)
	if r.settings.Report {
		data = r.parts.Views.Build(report, r.title, r.hidden(report))
	}
	body, err := r.parts.Renderer.Render(data)
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return nil
	}
	r.pending.Go(func() {
		if r.parts.Sleeper.Sleep(ctx, r.settings.Delay) == nil {
			r.parts.Sender.Send(ctx, r.title, body)
		}
	})
	return nil
}

func (r *Reporter) announceFailures(ctx context.Context, report domain.Report) {
	for _, result := range report.Results() {
		if result.State != domain.StateRollbackFailed {
			continue
		}
		body := fmt.Sprintf("error: rollback_failed (%s) for %s, transaction %s, snapshot %s; the stack is frozen until released with POST /v1/approve",
			result.Reason, result.Container, result.Transaction, result.Snapshot)
		r.pending.Go(func() { r.parts.Sender.Send(ctx, r.title, body) })
	}
}

func (r *Reporter) hidden(report domain.Report) func(domain.Result) bool {
	hidden, err := r.parts.Held.Hidden(report)
	if err != nil {
		r.parts.Logger.Error("cannot record notified held entries, reporting all of them", "cycle", report.CycleID, "error", err.Error())
		return func(domain.Result) bool { return false }
	}
	return hidden
}

func (r *Reporter) Announce(ctx context.Context, level domain.Level, body string) {
	if len(r.settings.URLs) == 0 || !r.parts.Views.Shows(level) {
		return
	}
	r.parts.Sender.Send(ctx, r.title, body)
}

func (r *Reporter) Wait() {
	r.pending.Wait()
}
