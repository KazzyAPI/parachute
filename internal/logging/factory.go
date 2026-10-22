package logging

import (
	"io"
	"log/slog"
	"time"

	"github.com/KazzyAPI/parachute/internal/config"
	"github.com/KazzyAPI/parachute/internal/domain"
)

type Factory struct {
	location *time.Location
	terminal bool
}

func NewFactory(location *time.Location, terminal bool) *Factory {
	return &Factory{location: location, terminal: terminal}
}

func (f *Factory) New(settings config.Log, out io.Writer) *slog.Logger {
	options := &slog.HandlerOptions{Level: domain.ParseLevel(settings.Level).Slog(), ReplaceAttr: f.replace}
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
			return slog.String(slog.LevelKey, domain.Level(level).String())
		}
	case slog.TimeKey:
		return slog.Time(slog.TimeKey, attribute.Value.Time().In(f.location))
	}
	return attribute
}

func (f *Factory) Time(t time.Time) string {
	return t.In(f.location).Format(time.RFC3339)
}
