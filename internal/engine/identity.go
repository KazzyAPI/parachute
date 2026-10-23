package engine

import (
	"context"
	"fmt"

	"github.com/moby/moby/client"
)

func (d *Docker) EngineID(ctx context.Context) (string, error) {
	info, err := d.client.Info(ctx, client.InfoOptions{})
	if err != nil {
		return "", fmt.Errorf("read engine information: %w", err)
	}
	return info.Info.ID, nil
}
