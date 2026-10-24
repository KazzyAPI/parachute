package app

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/drift"
	"github.com/KazzyAPI/parachute/internal/health"
	"github.com/KazzyAPI/parachute/internal/logging"
	"github.com/KazzyAPI/parachute/internal/redact"
	"github.com/KazzyAPI/parachute/internal/state"
)

type System interface {
	config.System
	config.FileSystem
	state.FileSystem
	health.FileSystem
	health.Process
	drift.FileReader
}

type Dependencies struct {
	Stdout   io.Writer
	Stderr   io.Writer
	System   System
	Clock    Clock
	Random   io.Reader
	Version  string
	Terminal bool
}

type App struct {
	deps Dependencies
}

func New(deps Dependencies) *App {
	return &App{deps: deps}
}

func (a *App) Run(ctx context.Context, args, environ []string) int {
	redactor := redact.New()
	cfg, err := a.loader().Load(args, environ)
	if err != nil {
		handler := logging.NewRedactingHandler(slog.NewTextHandler(a.deps.Stderr, nil), redactor, domain.LevelDebug.Slog())
		slog.New(handler).Error(err.Error())
		return domain.ExitConfiguration
	}
	redactor = redact.New(a.secretSources(cfg)...)
	stdout := redact.NewWriter(a.deps.Stdout, redactor)
	deps := a.deps
	deps.Stdout = stdout
	entries := logging.NewCollector(domain.ParseLevel(cfg.Notify.Level))
	var collectors []slog.Handler
	if !cfg.Notify.Report {
		collectors = append(collectors, entries)
	}
	logger := logging.NewFactory(cfg.Schedule.Location, a.deps.Terminal).New(cfg.Log, a.deps.Stderr, redactor, collectors...)
	if cfg.Log.Level == "trace" {
		logger.Warn("trace logging is on: trace lines are not redacted and can contain credentials and other secrets")
	}
	container := NewContainer(cfg, deps, logger, redactor, entries)
	err = a.dispatch(ctx, container)
	if flushErr := stdout.Flush(); flushErr != nil && err == nil {
		err = fmt.Errorf("write stdout: %w", flushErr)
	}
	if err != nil {
		logger.Error(err.Error())
		return exitCodeOf(err)
	}
	return domain.ExitSuccess
}

func (a *App) secretSources(cfg config.Config) []redact.Source {
	return []redact.Source{redact.Static(cfg.API.Token, cfg.API.StatusToken), redact.URLSecrets(cfg.Notify.URLs), registrySecrets(cfg, a.deps.System)}
}

func (a *App) dispatch(ctx context.Context, container *Container) error {
	switch container.cfg.Command {
	case config.CommandHealthcheck:
		return container.Healthcheck()
	case config.CommandStatus:
		return container.Status(ctx)
	case config.CommandRestoreVerify:
		return container.RestoreVerify(ctx)
	}
	daemon, closeEngine, err := container.Daemon(ctx)
	if err != nil {
		return err
	}
	runErr := daemon.Run(ctx)
	if closeErr := closeEngine(); closeErr != nil && runErr == nil {
		return fmt.Errorf("close Docker client: %w", closeErr)
	}
	return runErr
}

func (a *App) loader() *config.Loader {
	registry := config.NewRegistry(config.DefaultSettings())
	removed := config.NewRemovedSettings()
	suggester := config.NewSuggester(3)
	syntax := config.NewSyntax()
	return config.NewLoader(
		registry,
		config.NewArgumentParser(registry, removed, suggester),
		config.NewEnvironmentReader(registry, removed, suggester),
		config.NewResolver(registry, syntax, a.deps.System),
		a.deps.System,
	)
}
