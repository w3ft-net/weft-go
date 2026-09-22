package weft

import (
	"context"
	"log/slog"
)

// SlogHandler wraps an existing slog.Handler so records emitted
// through it gain a "trace_id" attribute from the ambient context,
// when present. Wrap once in main():
//
//	slog.SetDefault(slog.New(weft.NewSlogHandler(slog.NewJSONHandler(os.Stdout, nil))))
//
// Only records logged via a ctx-carrying call (Logger.InfoContext,
// ErrorContext, etc.) see a trace_id — slog.Info() with no context
// carries none.
//
// Caveat: if an upstream Logger.WithGroup call is active, trace_id
// nests under that group like any other attribute in the record —
// this is inherent to how slog handler groups render, not specific
// to this wrapper.
type SlogHandler struct {
	next slog.Handler
}

// NewSlogHandler wraps next so records gain a trace_id attribute
// from the ambient context, when present.
func NewSlogHandler(next slog.Handler) *SlogHandler {
	return &SlogHandler{next: next}
}

func (h *SlogHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.next.Enabled(ctx, level)
}

func (h *SlogHandler) Handle(ctx context.Context, record slog.Record) error {
	if traceID, ok := TraceIDFromContext(ctx); ok {
		record.AddAttrs(slog.String("trace_id", traceID))
	}
	return h.next.Handle(ctx, record)
}

func (h *SlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &SlogHandler{next: h.next.WithAttrs(attrs)}
}

func (h *SlogHandler) WithGroup(name string) slog.Handler {
	return &SlogHandler{next: h.next.WithGroup(name)}
}

var _ slog.Handler = (*SlogHandler)(nil)
