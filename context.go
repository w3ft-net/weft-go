package weft

import (
	"context"
	"strings"
)

// traceIDKey is the unexported context key carrying the ambient
// trace ID, mirroring weft's own store.orgCtxKey pattern.
type traceIDKey struct{}

// ContextWithTraceID returns a context carrying traceID as the
// ambient trace context. Client.Send/SendLine, TraceHeaders, and
// SlogHandler all read it back via TraceIDFromContext.
func ContextWithTraceID(ctx context.Context, traceID string) context.Context {
	return context.WithValue(ctx, traceIDKey{}, traceID)
}

// TraceIDFromContext reports the ambient trace ID, if any. An
// empty string is treated as absent.
func TraceIDFromContext(ctx context.Context) (string, bool) {
	v, ok := ctx.Value(traceIDKey{}).(string)
	return v, ok && v != ""
}

// TraceHeaders returns outbound propagation headers derived from
// ctx's ambient trace ID. Empty map if ctx carries none — there is
// nothing to propagate; TraceHeaders deliberately does not fabricate
// a fresh trace ID, since that would trace only this one hop with no
// link back to whatever called this process. Establish one first via
// TraceMiddleware or ContextWithTraceID(ctx, NewTraceID()).
func TraceHeaders(ctx context.Context) map[string]string {
	traceID, ok := TraceIDFromContext(ctx)
	if !ok {
		return map[string]string{}
	}
	return map[string]string{"traceparent": FormatTraceParent(traceID)}
}

// validTraceIDForEnvelope rejects control bytes that would corrupt
// weftd's socket \x1e envelope or line framing. NewTraceID/
// ParseTraceParent output is always safe (hex-only); this guards the
// path where a caller passes an arbitrary string to
// ContextWithTraceID directly.
func validTraceIDForEnvelope(id string) bool {
	return !strings.ContainsAny(id, "\x1e\n")
}
