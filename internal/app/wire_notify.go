package app

import (
	"errors"
	"io"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/notify"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/porcelain"
	"github.com/KazzyAPI/parachute/internal/state"
)

func (c *Container) reporter(store *state.Store) (*notify.Reporter, error) {
	settings := c.cfg.Notify
	if c.cfg.Policy.ReminderDays <= 0 {
		return nil, configurationError(errors.New("--approval-reminder-days must be a positive number of days"))
	}
	for _, rawURL := range settings.URLs {
		if err := notify.ValidateURL(rawURL); err != nil {
			return nil, configurationError(err)
		}
	}
	renderer, err := notify.NewRenderer(settings.Template, settings.Report)
	if err != nil {
		return nil, configurationError(err)
	}
	var stdout io.Writer
	if settings.LogStdout {
		stdout = c.deps.Stdout
	}
	return notify.NewReporter(settings, notify.ReporterParts{
		Sender:   notify.NewShoutrrrSender(settings.URLs, c.logger, c.redactor, stdout),
		Views:    notify.NewViewBuilder(domain.ParseLevel(settings.Level), settings.Hostname),
		Renderer: renderer,
		Sleeper:  c.deps.Clock,
		Held:     notify.NewLedger(store, c.cfg.Policy.ReminderDays),
		Entries:  c.entries,
	}), nil
}

func (c *Container) sinks(store *state.Store, reporter *notify.Reporter) []pipeline.Sink {
	sinks := []pipeline.Sink{reporter, state.NewRecorder(store)}
	if c.cfg.Porcelain == "v1" {
		sinks = append(sinks, porcelain.NewWriter(c.deps.Stdout))
	}
	return sinks
}
