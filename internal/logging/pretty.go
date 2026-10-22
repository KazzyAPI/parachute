package logging

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type PrettyHandler struct {
	out        io.Writer
	minimum    slog.Level
	color      bool
	times      *Factory
	attributes []slog.Attr
	lock       *sync.Mutex
}

func NewPrettyHandler(out io.Writer, minimum slog.Level, color bool, times *Factory) *PrettyHandler {
	return &PrettyHandler{out: out, minimum: minimum, color: color, times: times, lock: &sync.Mutex{}}
}

func (h *PrettyHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.minimum
}

func (h *PrettyHandler) Handle(_ context.Context, record slog.Record) error {
	var line strings.Builder
	level := strings.ToUpper(domain.Level(record.Level).String())
	fmt.Fprintf(&line, "%s %s %s", h.times.Time(record.Time), h.paint(record.Level, fmt.Sprintf("%-5s", level)), record.Message)
	write := func(attribute slog.Attr) bool {
		fmt.Fprintf(&line, " %s=%v", attribute.Key, attribute.Value)
		return true
	}
	for _, attribute := range h.attributes {
		write(attribute)
	}
	record.Attrs(write)
	line.WriteByte('\n')
	h.lock.Lock()
	defer h.lock.Unlock()
	_, err := io.WriteString(h.out, line.String())
	return err
}

func (h *PrettyHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	clone := *h
	clone.attributes = append(append([]slog.Attr{}, h.attributes...), attributes...)
	return &clone
}

func (h *PrettyHandler) WithGroup(_ string) slog.Handler {
	return h
}

func (h *PrettyHandler) paint(level slog.Level, text string) string {
	if !h.color {
		return text
	}
	code := "36"
	switch {
	case level >= slog.LevelError:
		code = "31"
	case level >= slog.LevelWarn:
		code = "33"
	case level >= slog.LevelInfo:
		code = "32"
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}
