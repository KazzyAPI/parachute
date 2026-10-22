package logging

import (
	"io"
	"log/slog"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
)

const LevelTrace = slog.Level(-8)

type Factory struct {
	location *time.Location
	terminal bool
}

func NewFactory(location *time.Location, terminal bool) *Factory {
	return &Factory{location: location, terminal: terminal}
}

func (f *Factory) Level(name string) slog.Level {
	switch name {
	case "trace":
		return LevelTrace
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	return slog.LevelInfo
}

func (f *Factory) LevelName(level slog.Level) string {
	switch {
	case level <= LevelTrace:
		return "trace"
	case level <= slog.LevelDebug:
		return "debug"
	case level <= slog.LevelInfo:
		return "info"
	case level <= slog.LevelWarn:
		return "warn"
	}
	return "error"
}

func (f *Factory) New(settings config.Log, out io.Writer) *slog.Logger {
	options := &slog.HandlerOptions{Level: f.Level(settings.Level), ReplaceAttr: f.replace}
	format := settings.Format
	if format == "auto" {
		format = "logfmt"
		if f.terminal {
			format = "pretty"
		}
	}
	switch format {
	case "json":
		return slog.New(slog.NewJSONHandler(out, options))
	case "pretty":
		return slog.New(NewPrettyHandler(out, options.Level.Level(), !settings.NoColor, f))
	}
	return slog.New(slog.NewTextHandler(out, options))
}

func (f *Factory) replace(groups []string, attribute slog.Attr) slog.Attr {
	if len(groups) > 0 {
		return attribute
	}
	switch attribute.Key {
	case slog.LevelKey:
		if level, ok := attribute.Value.Any().(slog.Level); ok {
			return slog.String(slog.LevelKey, f.LevelName(level))
		}
	case slog.TimeKey:
		return slog.Time(slog.TimeKey, attribute.Value.Time().In(f.location))
	}
	return attribute
}

func (f *Factory) Time(t time.Time) string {
	return t.In(f.location).Format(time.RFC3339)
}
