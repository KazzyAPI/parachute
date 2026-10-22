package app

import (
	"context"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/status"
)

func (c *Container) Status(ctx context.Context) (err error) {
	store := state.NewStore(c.cfg.StateDir, c.deps.System)
	if _, _, err := store.Read(); err != nil {
		return configurationError(err)
	}
	docker, err := c.docker()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := docker.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close Docker client: %w", closeErr)
		}
	}()
	if err := docker.Verify(ctx); err != nil {
		return unreachableError(fmt.Errorf("no managed host is reachable: %s: %w", c.cfg.Engine.Host, err))
	}
	builder := status.NewBuilder(localHost, docker, c.selector(), c.cfg.Armed)
	command := status.NewCommand(builder, store, c.deps.Clock, c.deps.Stdout)
	document, err := command.Document(ctx)
	if err != nil {
		return unreachableError(err)
	}
	if c.cfg.JSON {
		return command.WriteJSON(document)
	}
	return command.WriteTable(document)
}
