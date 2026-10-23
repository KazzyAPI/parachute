package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
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
	redactor = redact.New(redact.Static(cfg.API.Token, cfg.API.StatusToken))
	logger := logging.NewFactory(cfg.Schedule.Location, a.deps.Terminal).New(cfg.Log, a.deps.Stderr, redactor)
	container := NewContainer(cfg, a.deps, logger, redactor)
	if err := a.dispatch(ctx, container); err != nil {
		logger.Error(err.Error())
		return exitCodeOf(err)
	}
	return domain.ExitSuccess
}

func (a *App) dispatch(ctx context.Context, container *Container) error {
	switch container.cfg.Command {
	case config.CommandHealthcheck:
		return container.Healthcheck()
	case config.CommandStatus:
		return container.Status(ctx)
	case config.CommandRestoreVerify:
		return configurationError(errors.New("the restore-verify command is not implemented in this milestone"))
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
