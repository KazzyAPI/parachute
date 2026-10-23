package logging

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/KazzyAPI/parachute/internal/domain"
)

type Entry struct {
	Time    time.Time
	Level   string
	Message string
	Fields  map[string]string
}

type entries struct {
	lock  sync.Mutex
	items []Entry
}

type Collector struct {
	minimum    slog.Level
	shared     *entries
	attributes []slog.Attr
}

func NewCollector(minimum domain.Level) *Collector {
	return &Collector{minimum: minimum.Slog(), shared: &entries{}}
}

func (c *Collector) Enabled(_ context.Context, level slog.Level) bool { return level >= c.minimum }

func (c *Collector) Handle(_ context.Context, record slog.Record) error {
	entry := Entry{Time: record.Time, Level: domain.Level(record.Level).String(), Message: record.Message, Fields: map[string]string{}}
	add := func(attribute slog.Attr) bool {
		entry.Fields[attribute.Key] = attribute.Value.Resolve().String()
		return true
	}
	for _, attribute := range c.attributes {
		add(attribute)
	}
	record.Attrs(add)
	c.shared.lock.Lock()
	defer c.shared.lock.Unlock()
	c.shared.items = append(c.shared.items, entry)
	return nil
}

func (c *Collector) WithAttrs(attributes []slog.Attr) slog.Handler {
	clone := *c
	clone.attributes = append(slices.Clone(c.attributes), attributes...)
	return &clone
}

func (c *Collector) WithGroup(_ string) slog.Handler { return c }

func (c *Collector) Take(cycleID string) []Entry {
	c.shared.lock.Lock()
	defer c.shared.lock.Unlock()
	start := slices.IndexFunc(c.shared.items, func(entry Entry) bool { return entry.Fields["cycle"] == cycleID })
	if start < 0 {
		return nil
	}
	var taken, kept []Entry
	for index, entry := range c.shared.items {
		switch cycle := entry.Fields["cycle"]; {
		case cycle != "" && cycle != cycleID:
			kept = append(kept, entry)
		case index >= start:
			taken = append(taken, entry)
		}
	}
	c.shared.items = kept
	return taken
}

type Tee struct {
	handlers []slog.Handler
}

func NewTee(handlers ...slog.Handler) *Tee { return &Tee{handlers: handlers} }

func (t *Tee) Enabled(ctx context.Context, level slog.Level) bool {
	return slices.ContainsFunc(t.handlers, func(handler slog.Handler) bool { return handler.Enabled(ctx, level) })
}

func (t *Tee) Handle(ctx context.Context, record slog.Record) error {
	var failure error
	for _, handler := range t.handlers {
		if handler.Enabled(ctx, record.Level) {
			if err := handler.Handle(ctx, record.Clone()); err != nil && failure == nil {
				failure = err
			}
		}
	}
	return failure
}

func (t *Tee) WithAttrs(attributes []slog.Attr) slog.Handler {
	return t.each(func(handler slog.Handler) slog.Handler { return handler.WithAttrs(attributes) })
}

func (t *Tee) WithGroup(name string) slog.Handler {
	return t.each(func(handler slog.Handler) slog.Handler { return handler.WithGroup(name) })
}

func (t *Tee) each(derive func(slog.Handler) slog.Handler) slog.Handler {
	derived := make([]slog.Handler, 0, len(t.handlers))
	for _, handler := range t.handlers {
		derived = append(derived, derive(handler))
	}
	return &Tee{handlers: derived}
}
