package domain

import "log/slog"

type Level slog.Level

const (
	LevelTrace = Level(-8)
	LevelDebug = Level(slog.LevelDebug)
	LevelInfo  = Level(slog.LevelInfo)
	LevelWarn  = Level(slog.LevelWarn)
	LevelError = Level(slog.LevelError)
)

func ParseLevel(name string) Level {
	switch name {
	case "trace":
		return LevelTrace
	case "debug":
		return LevelDebug
	case "warn":
		return LevelWarn
	case "error":
		return LevelError
	}
	return LevelInfo
}

func (l Level) Slog() slog.Level { return slog.Level(l) }

func (l Level) String() string {
	switch {
	case l <= LevelTrace:
		return "trace"
	case l <= LevelDebug:
		return "debug"
	case l <= LevelInfo:
		return "info"
	case l <= LevelWarn:
		return "warn"
	}
	return "error"
}
