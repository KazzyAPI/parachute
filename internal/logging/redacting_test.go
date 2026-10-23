package logging

import (
	"bytes"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/KazzyAPI/parachute/internal/redact"
)

func TestRedactingHandlerSparesOnlyTraceLines(t *testing.T) {
	var out bytes.Buffer
	inner := slog.NewTextHandler(&out, &slog.HandlerOptions{Level: slog.Level(-8)})
	logger := slog.New(NewRedactingHandler(inner, redact.New(redact.Static("hunter22")), slog.LevelDebug)).With("token", "hunter22")
	logger.Info("sent hunter22", "error", errors.New("dial hunter22"), slog.Group("g", "url", "https://u:p@h"))
	if strings.Contains(out.String(), "hunter22") || strings.Contains(out.String(), "u:p") {
		t.Errorf("secret logged: %s", out.String())
	}
	out.Reset()
	slog.New(NewRedactingHandler(inner, redact.New(redact.Static("hunter22")), slog.LevelDebug)).Log(t.Context(), slog.Level(-8), "hunter22")
	if !strings.Contains(out.String(), "hunter22") {
		t.Errorf("trace line redacted: %s", out.String())
	}
}
