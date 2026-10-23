package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/health"
	"github.com/KazzyAPI/parachute/internal/labels"
	"github.com/KazzyAPI/parachute/internal/notify"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/schedule"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/status"
)

type Container struct {
	cfg    config.Config
	deps   Dependencies
	logger *slog.Logger
	labels *labels.Reader
}

func NewContainer(cfg config.Config, deps Dependencies, logger *slog.Logger) *Container {
	return &Container{cfg: cfg, deps: deps, logger: logger, labels: labels.NewReader(logger)}
}

func (c *Container) cyclePipeline(store *state.Store, reporter *notify.Reporter) *pipeline.Pipeline {
	return pipeline.New(
		pipeline.NewDiscoverStage(c.logger),
		c.selectStage(),
		c.detectStage(),
		c.policyStage(store),
		pipeline.NewReportStage(c.deps.Clock, c.logger, c.sinks(store, reporter)...),
	)
}

func (c *Container) Daemon(ctx context.Context) (*Daemon, func() error, error) {
	store := state.NewStore(c.cfg.StateDir, c.deps.System)
	if err := store.Open(); err != nil {
		return nil, nil, configurationError(err)
	}
	trigger, err := c.trigger()
	if err != nil {
		return nil, nil, err
	}
	reporter, err := c.reporter()
	if err != nil {
		return nil, nil, err
	}
	hosts, closeHosts, err := c.hosts(ctx)
	if err != nil {
		return nil, nil, err
	}
	runner := NewRunner(c.cyclePipeline(store, reporter), hosts, c.cfg.Armed, c.deps.Clock, c.logger)
	gate := schedule.NewGate(schedule.NewCycleIDs(c.deps.Random), c.deps.Clock)
	var loop CycleLoop = schedule.NewScheduler(trigger, c.deps.Clock, gate, runner, c.logger, c.cfg.Schedule.Location)
	scheduleText := trigger.Describe()
	if c.cfg.RunOnce {
		loop, scheduleText = NewSingleCycle(gate, runner), "run once"
	}
	server, err := c.server(ServerParts{Gate: gate, Runner: runner, Store: store, Status: c.statusSource(hosts), Hosts: hosts})
	if err != nil {
		return nil, nil, errors.Join(err, closeHosts())
	}
	return NewDaemon(DaemonParts{
		Guard:          NewArmedLabelGuard(hosts[0].Source, c.deps.System, c.labels),
		Announcer:      reporter,
		Marker:         health.NewMarker(c.cfg.RuntimeDir, c.deps.System, c.deps.System),
		Loop:           loop,
		Server:         server,
		Summary:        StartupSummary{Version: c.deps.Version, Hosts: hosts, Schedule: scheduleText, Settings: c.cfg},
		StartupMessage: c.cfg.StartupMessage,
		Logger:         c.logger,
	}), closeHosts, nil
}

func (c *Container) Status(ctx context.Context) (err error) {
	saved, _, err := state.NewStore(c.cfg.StateDir, c.deps.System).Read()
	if err != nil {
		return configurationError(err)
	}
	hosts, closeHosts, err := c.hosts(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeHosts(); closeErr != nil && err == nil {
			err = fmt.Errorf("close Docker client: %w", closeErr)
		}
	}()
	command := status.NewCommand(c.statusSource(hosts), c.deps.Clock, c.deps.Stdout)
	document, err := command.Document(ctx, saved)
	if err != nil {
		return unreachableError(err)
	}
	if c.cfg.JSON {
		return command.WriteJSON(document)
	}
	return command.WriteTable(document)
}
