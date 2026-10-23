package notify

import (
	"context"
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
	if r.settings.Report {
		hidden, err := r.parts.Held.Hidden(report)
		if err != nil {
			return err
		}
		data = r.parts.Views.Build(report, r.title, hidden)
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

func (r *Reporter) Announce(ctx context.Context, level domain.Level, body string) {
	if len(r.settings.URLs) == 0 || !r.parts.Views.Shows(level) {
		return
	}
	r.parts.Sender.Send(ctx, r.title, body)
}

func (r *Reporter) Wait() {
	r.pending.Wait()
}
