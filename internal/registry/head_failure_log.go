package registry

import (
	"context"
	"log/slog"
)

type HeadFailureLog struct {
	mode   string
	logger *slog.Logger
	loud   map[string]bool
}

func NewHeadFailureLog(mode string, logger *slog.Logger) *HeadFailureLog {
	return &HeadFailureLog{mode: mode, logger: logger, loud: map[string]bool{
		"docker.io": true, "index.docker.io": true, "registry-1.docker.io": true, "ghcr.io": true,
	}}
}

func (h *HeadFailureLog) HeadFailed(ctx context.Context, ref Reference, err error) {
	level := slog.LevelDebug
	if h.mode == "always" || h.mode == "auto" && h.loud[ref.Domain] {
		level = slog.LevelWarn
	}
	h.logger.Log(ctx, level, "manifest HEAD request failed, falling back to GET", "registry", ref.Domain, "error", err.Error())
}
