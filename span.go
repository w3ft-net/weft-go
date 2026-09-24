package weft

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log"
	"time"
)

// spanIDKey is the unexported context key carrying the ambient span
// ID — the span a nested StartSpan call, or an outbound RPC's own
// courier metadata, should record as its parent. Separate from
// traceIDKey: a context can carry a trace ID with no ambient span
// (e.g. right after TraceMiddleware, before any StartSpan call).
type spanIDKey struct{}

// NewSpanID returns a fresh 16-lowercase-hex-char span ID (8 random
// bytes via crypto/rand, matching the W3C traceparent parent-id
// segment's length — see FormatTraceParent/ParseTraceParent for the
// external, HTTP-boundary propagation path, and weft's own
// weft-span-id gRPC metadata courier for weft-to-weft hops).
func NewSpanID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic("weft: crypto/rand unavailable: " + err.Error())
	}
	return hex.EncodeToString(b[:])
}

// CurrentSpanID reports ctx's ambient span ID, if any — the span a
// nested StartSpan call, or a courier propagating this process's
// call to another, should record as the parent.
func CurrentSpanID(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(spanIDKey{}).(string)
	return v, ok && v != ""
}

// ContextWithSpanID returns a context carrying spanID as the ambient
// span. Exported for the courier side of a process boundary: a
// server reading a parent span ID out of its own metadata (mirroring
// ContextWithTraceID) calls this before StartSpan so the span it
// creates parents correctly.
func ContextWithSpanID(ctx context.Context, spanID string) context.Context {
	return context.WithValue(ctx, spanIDKey{}, spanID)
}

// Span is one timed unit of work within a trace. Obtained from
// StartSpan; call `defer span.End()` immediately after and nothing
// else at the call site — RecordError, not a separate End variant,
// is how an error gets attached, so a single unconditional defer is
// always correct regardless of how many return paths the function
// has.
type Span struct {
	traceID  string
	spanID   string
	parentID string
	name     string
	start    time.Time
	ended    bool
	hadError bool
}

// StartSpan begins a new span named name, parented off ctx's ambient
// span if any, under ctx's ambient trace ID. Returns a context
// carrying the new span as ambient — pass it to any downstream call
// so a nested StartSpan (same process) parents off this span, or a
// courier reading CurrentSpanID before an outbound RPC ships the
// right parent.
//
// No-ops when ctx carries no trace ID: the returned Span's End/
// RecordError do nothing, and the returned context is ctx unchanged.
// This mirrors TraceHeaders' "don't fabricate what nothing asked for"
// rule — it's what lets a callsite start/end a span unconditionally
// instead of wrapping it in its own `if traced { ... }` gate.
//
// Logs a "span start" line immediately (not just "span done" at End)
// so a request that hangs forever still leaves a visible mark of
// which layer it reached and never returned from.
func StartSpan(ctx context.Context, name string) (context.Context, *Span) {
	traceID, ok := TraceIDFromContext(ctx)
	if !ok {
		return ctx, &Span{}
	}
	parentID, _ := CurrentSpanID(ctx)
	s := &Span{
		traceID:  traceID,
		spanID:   NewSpanID(),
		parentID: parentID,
		name:     name,
		start:    time.Now(),
	}
	log.Printf("span start name=%s trace_id=%s span_id=%s parent_span_id=%s", s.name, s.traceID, s.spanID, s.parentID)
	return ContextWithSpanID(ctx, s.spanID), s
}

// RecordError marks the span as having encountered err, logging it
// immediately (not buffered until End) so a request that hangs after
// this point still leaves the error visible. Call it wherever an
// error is actually discovered — a failed sub-call, a bad response —
// not just at a function's final return. Safe to call more than once
// (a span can encounter several errors and still eventually
// succeed, e.g. inside a retry loop) and safe on a no-op Span. Does
// not end the span; that's still End's job alone, via defer, exactly
// once, unconditionally — the same separation OpenTelemetry's
// RecordError/SetStatus vs. End makes, and for the same reason: by
// the time a function returns, an error it handled and recovered
// from is indistinguishable from one that never happened unless it
// was recorded when it actually occurred.
func (s *Span) RecordError(err error) {
	if s == nil || s.traceID == "" || err == nil {
		return
	}
	s.hadError = true
	log.Printf("span error name=%s trace_id=%s span_id=%s err=%v", s.name, s.traceID, s.spanID, err)
}

// End logs the span's completion, with duration and whether
// RecordError was ever called on it. Call exactly once, via
// `defer span.End()` immediately after StartSpan — no branching, no
// separate error variant needed at the call site.
func (s *Span) End() {
	if s == nil || s.traceID == "" || s.ended {
		return
	}
	s.ended = true
	duration := time.Since(s.start)
	status := "ok"
	if s.hadError {
		status = "error"
	}
	log.Printf("span done name=%s trace_id=%s span_id=%s parent_span_id=%s duration=%s status=%s",
		s.name, s.traceID, s.spanID, s.parentID, duration, status)
}
