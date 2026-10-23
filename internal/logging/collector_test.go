package logging

import (
	"log/slog"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
)

func TestCollectorTakesTheCycleEntriesAtOrAboveItsLevel(t *testing.T) {
	collector := NewCollector(domain.LevelInfo)
	logger := slog.New(NewTee(slog.DiscardHandler, collector))
	logger.Info("before")
	logger.Info("cycle started", "cycle", "c1")
	logger.Debug("hidden")
	logger.With("host", "local").Warn("checked", "cycle", "c1")
	entries := collector.Take("c1")
	if len(entries) != 2 || entries[0].Message != "cycle started" || entries[1].Fields["host"] != "local" || entries[1].Level != "warn" {
		t.Errorf("entries %+v", entries)
	}
	if again := collector.Take("c1"); again != nil {
		t.Errorf("entries kept after take: %+v", again)
	}
}
