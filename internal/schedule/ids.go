package schedule

import (
	"encoding/hex"
	"fmt"
	"io"
	"time"
)

type CycleIDs struct {
	random io.Reader
}

func NewCycleIDs(random io.Reader) *CycleIDs {
	return &CycleIDs{random: random}
}

func (g *CycleIDs) New(started time.Time) (string, error) {
	suffix := make([]byte, 3)
	if _, err := io.ReadFull(g.random, suffix); err != nil {
		return "", fmt.Errorf("generate cycle ID: %w", err)
	}
	return "cyc-" + started.UTC().Format("20060102T150405Z") + "-" + hex.EncodeToString(suffix), nil
}
