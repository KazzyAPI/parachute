package app

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/KazzyAPI/parachute/internal/clock"
	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/logging"
	"github.com/KazzyAPI/parachute/internal/osys"
)

type Dependencies struct {
	Stdout   io.Writer
	Stderr   io.Writer
	System   *osys.System
	Clock    *clock.System
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
	cfg, err := a.loader().Load(args, environ)
	if err != nil {
		fmt.Fprintf(a.deps.Stderr, "parachute: %v\n", err)
		return ExitConfiguration
	}
	logger := logging.NewFactory(cfg.Schedule.Location, a.deps.Terminal).New(cfg.Log, a.deps.Stderr)
	container := NewContainer(cfg, a.deps, logger)
	if err := a.dispatch(ctx, container); err != nil {
		logger.Error(err.Error())
		return exitCodeOf(err)
	}
	return ExitSuccess
}

func (a *App) dispatch(ctx context.Context, container *Container) error {
	switch container.cfg.Command {
	case "healthcheck":
		return container.Healthcheck()
	case "status":
		return container.Status(ctx)
	case "restore-verify":
		return configurationError(errors.New("the restore-verify command is not implemented in this milestone"))
	}
	daemon, closeEngine, err := container.Daemon()
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
		syntax,
		a.deps.System,
	)
}
