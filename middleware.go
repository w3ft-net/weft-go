package weft

import "net/http"

// TraceMiddleware establishes ambient trace context for a request:
// adopts the trace-id segment of an inbound traceparent header if
// well-formed, otherwise generates one. Downstream handlers read it
// via TraceIDFromContext(r.Context()). Echoes Traceparent on the
// response so a caller (or a log-correlating proxy) can observe the
// ID actually used, including the generated-fresh case.
//
// A plain func(http.Handler) http.Handler, so it works unmodified
// with net/http, chi, gorilla/mux, or any router that shares this
// convention.
func TraceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID, ok := ParseTraceParent(r.Header.Get("traceparent"))
		if !ok {
			traceID = NewTraceID()
		}
		w.Header().Set("Traceparent", FormatTraceParent(traceID))
		next.ServeHTTP(w, r.WithContext(ContextWithTraceID(r.Context(), traceID)))
	})
}
