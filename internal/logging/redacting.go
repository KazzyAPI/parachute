package logging

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/KazzyAPI/parachute/internal/redact"
)

type RedactingHandler struct {
	inner    slog.Handler
	redactor *redact.Redactor
	minimum  slog.Level
}

func NewRedactingHandler(inner slog.Handler, r *redact.Redactor, level slog.Level) *RedactingHandler {
	return &RedactingHandler{inner: inner, redactor: r, minimum: level}
}

func (h *RedactingHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.inner.Enabled(ctx, level)
}

func (h *RedactingHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level < h.minimum {
		return h.inner.Handle(ctx, record)
	}
	redacted := slog.NewRecord(record.Time, record.Level, h.redactor.Redact(record.Message), record.PC)
	record.Attrs(func(attribute slog.Attr) bool {
		redacted.AddAttrs(h.attribute(attribute))
		return true
	})
	return h.inner.Handle(ctx, redacted)
}

func (h *RedactingHandler) WithAttrs(attributes []slog.Attr) slog.Handler {
	redacted := make([]slog.Attr, 0, len(attributes))
	for _, attribute := range attributes {
		redacted = append(redacted, h.attribute(attribute))
	}
	return &RedactingHandler{inner: h.inner.WithAttrs(redacted), redactor: h.redactor, minimum: h.minimum}
}

func (h *RedactingHandler) WithGroup(name string) slog.Handler {
	return &RedactingHandler{inner: h.inner.WithGroup(name), redactor: h.redactor, minimum: h.minimum}
}

func (h *RedactingHandler) attribute(attribute slog.Attr) slog.Attr {
	value := attribute.Value.Resolve()
	switch value.Kind() {
	case slog.KindString:
		return slog.String(attribute.Key, h.redactor.Redact(value.String()))
	case slog.KindGroup:
		group := value.Group()
		redacted := make([]any, 0, len(group))
		for _, member := range group {
			redacted = append(redacted, h.attribute(member))
		}
		return slog.Group(attribute.Key, redacted...)
	case slog.KindAny:
		return slog.String(attribute.Key, h.redactor.Redact(fmt.Sprint(value.Any())))
	}
	return slog.Attr{Key: attribute.Key, Value: value}
}
