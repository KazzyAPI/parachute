package app

import (
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/notify"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/porcelain"
	"github.com/KazzyAPI/parachute/internal/state"
)

func (c *Container) reporter() (*notify.Reporter, error) {
	renderer, err := notify.NewRenderer(c.cfg.Notify.Template)
	if err != nil {
		return nil, configurationError(err)
	}
	sender := notify.NewShoutrrrSender(c.cfg.Notify.URLs, c.logger)
	views := notify.NewViewBuilder(domain.ParseLevel(c.cfg.Notify.Level), c.cfg.Notify.Hostname)
	return notify.NewReporter(c.cfg.Notify, sender, views, renderer, c.deps.Clock), nil
}

func (c *Container) sinks(store *state.Store, reporter *notify.Reporter) []pipeline.Sink {
	sinks := []pipeline.Sink{reporter, state.NewRecorder(store)}
	if c.cfg.Porcelain == "v1" {
		sinks = append(sinks, porcelain.NewWriter(c.deps.Stdout))
	}
	return sinks
}
