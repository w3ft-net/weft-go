package weft

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTraceMiddleware_AdoptsValidInboundHeader(t *testing.T) {
	const validID = "0123456789abcdef0123456789abcdef"
	var gotID string
	var gotOK bool

	h := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotOK = TraceIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "00-"+validID+"-0000000000000000-01")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !gotOK || gotID != validID {
		t.Errorf("downstream saw (%q, %v), want (%q, true)", gotID, gotOK, validID)
	}
	if got := rec.Header().Get("Traceparent"); got != "00-"+validID+"-0000000000000000-01" {
		t.Errorf("response Traceparent = %q, want echoed header", got)
	}
}

func TestTraceMiddleware_GeneratesWhenMalformed(t *testing.T) {
	var gotID string
	var gotOK bool

	h := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotOK = TraceIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "not-a-real-header")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !gotOK || !hexTraceIDRe.MatchString(gotID) {
		t.Errorf("downstream saw (%q, %v), want a freshly generated 32-hex id", gotID, gotOK)
	}
}

func TestTraceMiddleware_GeneratesWhenAbsent(t *testing.T) {
	var gotID string
	var gotOK bool

	h := TraceMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotID, gotOK = TraceIDFromContext(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if !gotOK || !hexTraceIDRe.MatchString(gotID) {
		t.Errorf("downstream saw (%q, %v), want a freshly generated 32-hex id", gotID, gotOK)
	}
	if got := rec.Header().Get("Traceparent"); got == "" {
		t.Error("response Traceparent header not set")
	}
}
