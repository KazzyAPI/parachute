package notify

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
)

type Sender interface {
	Send(ctx context.Context, title, body string)
}

type Sleeper interface {
	Sleep(ctx context.Context, duration time.Duration) error
}

type Reporter struct {
	settings config.Notify
	sender   Sender
	views    *ViewBuilder
	renderer *Renderer
	sleeper  Sleeper
	title    string
	pending  sync.WaitGroup
}

func NewReporter(settings config.Notify, sender Sender, views *ViewBuilder, renderer *Renderer, sleeper Sleeper) *Reporter {
	title := "Parachute on " + settings.Hostname
	if settings.TitleTag != "" {
		title = "[" + settings.TitleTag + "] " + title
	}
	if settings.SkipTitle {
		title = ""
	}
	return &Reporter{settings: settings, sender: sender, views: views, renderer: renderer, sleeper: sleeper, title: title}
}

func (r *Reporter) Publish(ctx context.Context, report domain.Report) error {
	if len(r.settings.URLs) == 0 {
		return nil
	}
	body, err := r.renderer.Render(r.views.Build(report, r.title))
	if err != nil {
		return err
	}
	if strings.TrimSpace(body) == "" {
		return nil
	}
	r.pending.Go(func() {
		if r.sleeper.Sleep(ctx, r.settings.Delay) == nil {
			r.sender.Send(ctx, r.title, body)
		}
	})
	return nil
}

func (r *Reporter) Announce(ctx context.Context, level domain.Level, body string) {
	if len(r.settings.URLs) == 0 || !r.views.Shows(level) {
		return
	}
	r.sender.Send(ctx, r.title, body)
}

func (r *Reporter) Wait() {
	r.pending.Wait()
}
