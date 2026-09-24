package weft

import "net/http"

// TraceMiddleware establishes ambient trace context for a request:
// adopts the trace-id segment of an inbound traceparent header if
// well-formed, otherwise generates one. When the header also carries
// a span-id, it's seeded as the ambient span too — so a StartSpan
// call inside the handler parents off the calling system's span
// instead of starting a fresh root. Downstream handlers read the
// trace ID via TraceIDFromContext(r.Context()) and the span via
// CurrentSpanID(r.Context()). Echoes Traceparent on the response,
// round-tripping the same span-id it received (or the all-zero
// placeholder if none) — middleware wraps the handler, so by the
// time it can write the response the handler's own StartSpan-derived
// context has already gone out of scope; the echo confirms what was
// ambient at entry, not whatever the handler did internally.
//
// A plain func(http.Handler) http.Handler, so it works unmodified
// with net/http, chi, gorilla/mux, or any router that shares this
// convention.
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID, spanID, ok := ParseTraceParent(r.Header.Get("traceparent"))
		if !ok {
			traceID, spanID = NewTraceID(), ""
		}
		w.Header().Set("Traceparent", FormatTraceParent(traceID, spanID))
		ctx := ContextWithTraceID(r.Context(), traceID)
		if spanID != "" {
			ctx = ContextWithSpanID(ctx, spanID)
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
