package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/health"
	"github.com/KazzyAPI/parachute/internal/labels"
	"github.com/KazzyAPI/parachute/internal/logging"
	"github.com/KazzyAPI/parachute/internal/metrics"
	"github.com/KazzyAPI/parachute/internal/notify"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/redact"
	"github.com/KazzyAPI/parachute/internal/schedule"
	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/status"
)

type Container struct {
	cfg      config.Config
	deps     Dependencies
	logger   *slog.Logger
	redactor *redact.Redactor
	entries  *logging.Collector
	labels   *labels.Reader
}

func NewContainer(cfg config.Config, deps Dependencies, logger *slog.Logger, redactor *redact.Redactor, entries *logging.Collector) *Container {
	return &Container{cfg: cfg, deps: deps, logger: logger, redactor: redactor, entries: entries, labels: labels.NewReader(logger)}
}

func (c *Container) cyclePipeline(store *state.Store, reporter *notify.Reporter, collector *metrics.Collector, policyStage pipeline.Stage, hosts []pipeline.Host, dockers []*engine.Docker) *pipeline.Pipeline {
	return pipeline.New(
		c.insecureHostWarning(),
		c.discoverStage(),
		c.selectStage(),
		c.detectStage(),
		policyStage,
		c.driftStage(),
		c.updateStage(store, hosts, dockers),
		pipeline.NewReportStage(c.deps.Clock, c.logger, append(c.sinks(store, reporter), collector)...),
	)
}

func (c *Container) Daemon(ctx context.Context) (*Daemon, func() error, error) {
	store := state.NewStore(c.cfg.StateDir, c.deps.System, c.logger)
	if err := store.Open(); err != nil {
		return nil, nil, configurationError(err)
	}
	trigger, err := c.trigger()
	if err != nil {
		return nil, nil, err
	}
	reporter, err := c.reporter(store)
	if err != nil {
		return nil, nil, err
	}
	policyStage, err := c.policyStage(store)
	if err != nil {
		return nil, nil, err
	}
	hosts, dockers, closeHosts, err := c.hosts(ctx)
	if err != nil {
		return nil, nil, err
	}
	collector := metrics.NewCollector(hostNames(hosts))
	runner := NewRunner(c.cyclePipeline(store, reporter, collector, policyStage, hosts, dockers), hosts, c.cfg.Armed, c.deps.Clock, c.logger)
	gate := schedule.NewGate(schedule.NewCycleIDs(c.deps.Random), c.deps.Clock, collector)
	var loop CycleLoop = schedule.NewScheduler(trigger, c.deps.Clock, gate, runner, c.logger, c.cfg.Schedule.Location)
	scheduleText := trigger.Describe()
	if c.cfg.RunOnce {
		loop, scheduleText = NewSingleCycle(gate, runner), "run once"
	} else if c.apiTriggered() {
		loop, scheduleText = requestsOnly{}, "on HTTP API requests"
	}
	server, err := c.server(ServerParts{Gate: gate, Runner: runner, Store: store, Status: c.statusSource(hosts), Hosts: hosts, Metrics: collector})
	if err != nil {
		return nil, nil, errors.Join(err, closeHosts())
	}
	return NewDaemon(DaemonParts{
		Guard:          NewArmedLabelGuard(firstReachable(hosts), c.self(), c.labels),
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
	saved, _, err := state.NewStore(c.cfg.StateDir, c.deps.System, c.logger).Read()
	if err != nil {
		return configurationError(err)
	}
	hosts, _, closeHosts, err := c.hosts(ctx)
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

func hostNames(hosts []pipeline.Host) []string {
	names := make([]string, len(hosts))
	for i, host := range hosts {
		names[i] = host.Name
	}
	return names
}
