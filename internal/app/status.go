package app

import (
	"context"
	"fmt"

	"github.com/KazzyAPI/parachute/internal/state"
	"github.com/KazzyAPI/parachute/internal/status"
)

func (c *Container) Status(ctx context.Context) (err error) {
	saved, _, err := state.NewStore(c.cfg.StateDir, c.deps.System).Read()
	if err != nil {
		return configurationError(err)
	}
	docker, closeDocker, err := c.verifiedDocker(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := closeDocker(); closeErr != nil && err == nil {
			err = fmt.Errorf("close Docker client: %w", closeErr)
		}
	}()
	command := status.NewCommand(status.NewBuilder(localHost, docker, c.selector(), c.cfg.Armed), c.deps.Clock, c.deps.Stdout)
	document, err := command.Document(ctx, saved)
	if err != nil {
		return unreachableError(err)
	}
	if c.cfg.JSON {
		return command.WriteJSON(document)
	}
	return command.WriteTable(document)
}
