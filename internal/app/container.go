package app

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/detect"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/engine"
	"github.com/KazzyAPI/parachute/internal/health"
	"github.com/KazzyAPI/parachute/internal/labels"
	"github.com/KazzyAPI/parachute/internal/notify"
	"github.com/KazzyAPI/parachute/internal/pipeline"
	"github.com/KazzyAPI/parachute/internal/porcelain"
	"github.com/KazzyAPI/parachute/internal/registry"
	"github.com/KazzyAPI/parachute/internal/schedule"
	"github.com/KazzyAPI/parachute/internal/selection"
	"github.com/KazzyAPI/parachute/internal/state"
)

const localHost = "local"

type Container struct {
	cfg    config.Config
	deps   Dependencies
	logger *slog.Logger
	labels *labels.Reader
}

func NewContainer(cfg config.Config, deps Dependencies, logger *slog.Logger) *Container {
	return &Container{cfg: cfg, deps: deps, logger: logger, labels: labels.NewReader(logger)}
}

func (c *Container) userAgent() string { return "parachute/" + c.deps.Version }

func (c *Container) selector() *selection.Selector {
	return selection.NewSelector(c.cfg.Selection, c.labels, c.logger)
}

func (c *Container) docker() (*engine.Docker, error) {
	endpoint := engine.Endpoint{
		Host: c.cfg.Engine.Host, APIVersion: c.cfg.Engine.APIVersion, TLSVerify: c.cfg.Engine.TLSVerify,
		CertPath: c.cfg.Engine.CertPath, UserAgent: c.userAgent(),
	}
	if _, err := endpoint.Scheme(); err != nil {
		return nil, configurationError(err)
	}
	if endpoint.Plaintext() {
		c.logger.Warn("Docker host uses tcp:// without TLS verification; anyone who can reach this port controls the host", "host", endpoint.Host)
	}
	docker, err := engine.NewDocker(endpoint)
	if err != nil {
		return nil, unreachableError(err)
	}
	return docker, nil
}

func (c *Container) trigger() (schedule.Trigger, error) {
	if c.cfg.Schedule.Cron == "" {
		return schedule.NewIntervalTrigger(c.cfg.Schedule.Interval), nil
	}
	trigger, err := schedule.NewCronTrigger(c.cfg.Schedule.Cron, c.cfg.Schedule.Location)
	if err != nil {
		return nil, configurationError(err)
	}
	return trigger, nil
}

func (c *Container) reporter() (*notify.Reporter, error) {
	renderer, err := notify.NewRenderer(c.cfg.Notify.Template)
	if err != nil {
		return nil, configurationError(err)
	}
	minimum := domain.ParseLevel(c.cfg.Notify.Level)
	sender := notify.NewShoutrrrSender(c.cfg.Notify.URLs, c.logger)
	views := notify.NewViewBuilder(minimum, c.cfg.Notify.Hostname)
	return notify.NewReporter(c.cfg.Notify, sender, views, renderer, c.deps.Clock, minimum), nil
}

func (c *Container) host(source *engine.Retrying) pipeline.Host {
	httpClient := &http.Client{Timeout: 30 * time.Second}
	authenticator := registry.NewAuthenticator(httpClient, c.userAgent())
	headLog := registry.NewHeadFailureLog(c.cfg.Detection.WarnOnHeadFailure, c.logger)
	registryClient := registry.NewClient(httpClient, authenticator, c.userAgent(), headLog)
	return pipeline.Host{Name: localHost, Endpoint: c.cfg.Engine.Host, Source: source, Detector: detect.NewDetector(source, registryClient)}
}

func (c *Container) cyclePipeline(store *state.Store, reporter *notify.Reporter) *pipeline.Pipeline {
	sinks := []pipeline.Sink{reporter, state.NewRecorder(store)}
	if c.cfg.Porcelain == "v1" {
		sinks = append(sinks, porcelain.NewWriter(c.deps.Stdout))
	}
	selectionSettings := c.cfg.Selection
	return pipeline.New(
		pipeline.NewDiscoverStage(c.logger),
		pipeline.NewSelectStage(c.selector()),
		pipeline.NewDetectStage(
			registry.NewCredentialStore(c.cfg.Detection.DockerConfigDir, c.deps.System), c.labels,
			pipeline.SwitchSettings{Global: selectionSettings.NoPull, LabelWins: selectionSettings.LabelTakePrecedence}, c.logger),
		pipeline.NewClassifyStage(c.labels,
			pipeline.SwitchSettings{Global: selectionSettings.MonitorOnly, LabelWins: selectionSettings.LabelTakePrecedence}),
		pipeline.NewReportStage(c.deps.Clock, c.logger, sinks...),
	)
}

func (c *Container) Daemon() (*Daemon, func() error, error) {
	store := state.NewStore(c.cfg.StateDir, c.deps.System)
	if _, err := store.Open(); err != nil {
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
	docker, err := c.docker()
	if err != nil {
		return nil, nil, err
	}
	retrying := engine.NewRetrying(docker, c.deps.Clock, engine.RetryPolicy{
		FirstDelay: time.Second, MaxDelay: 30 * time.Second, Window: 5 * time.Minute, AttemptTimeout: time.Minute,
	}, c.logger)
	hosts := []pipeline.Host{c.host(retrying)}
	runner := NewRunner(c.cyclePipeline(store, reporter), hosts, c.cfg.Armed, c.deps.Clock, c.logger)
	ids := schedule.NewCycleIDs(c.deps.Random)
	scheduleText := trigger.Describe()
	if c.cfg.RunOnce {
		scheduleText = "run once"
	}
	return NewDaemon(DaemonParts{
		Engine:         docker,
		Endpoint:       c.cfg.Engine.Host,
		Announcer:      reporter,
		Marker:         health.NewMarker(c.cfg.RuntimeDir, c.deps.System, c.deps.System),
		Once:           runner,
		Loop:           schedule.NewScheduler(trigger, c.deps.Clock, ids, runner, c.logger, c.cfg.Schedule.Location),
		IDs:            ids,
		Clock:          c.deps.Clock,
		Hostname:       c.deps.System,
		Labels:         c.labels,
		Summary:        StartupSummary{Version: c.deps.Version, Hosts: hosts, Schedule: scheduleText, Settings: c.cfg},
		RunOnce:        c.cfg.RunOnce,
		StartupMessage: c.cfg.StartupMessage,
		Logger:         c.logger,
	}), docker.Close, nil
}

func (c *Container) Healthcheck() error {
	if health.NewMarker(c.cfg.RuntimeDir, c.deps.System, c.deps.System).Healthy() {
		return nil
	}
	return &ExitError{Code: ExitFailures, Err: errors.New("no ready Parachute daemon is running")}
}
