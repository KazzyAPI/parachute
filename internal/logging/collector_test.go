package logging

import (
	"log/slog"
	"testing"

	"github.com/KazzyAPI/parachute/internal/domain"
	"github.com/KazzyAPI/parachute/internal/redact"
)

func TestCollectorTakesTheCycleEntriesAtOrAboveItsLevel(t *testing.T) {
	collector := NewCollector(domain.LevelInfo)
	logger := slog.New(NewRedactingHandler(NewTee(slog.DiscardHandler, collector), redact.New(redact.Static("hunter22")), domain.LevelDebug.Slog()))
	logger.Info("before")
	logger.Info("cycle started", "cycle", "c1")
	logger.Info("other cycle", "cycle", "c2")
	logger.Debug("hidden")
	logger.With("host", "local").Warn("checked hunter22", "cycle", "c1", "detail", "hunter22")
	entries := collector.Take("c1")
	if len(entries) != 2 || entries[0].Message != "cycle started" || entries[1].Fields["host"] != "local" || entries[1].Level != "warn" {
		t.Fatalf("entries %+v", entries)
	}
	if entries[1].Message != "checked [REDACTED]" || entries[1].Fields["detail"] != "[REDACTED]" {
		t.Errorf("entry not redacted: %+v", entries[1])
	}
	if again := collector.Take("c2"); len(again) != 1 || again[0].Message != "other cycle" {
		t.Errorf("other cycle entries %+v", again)
	}
}
