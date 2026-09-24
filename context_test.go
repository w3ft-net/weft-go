package weft

import (
	"context"
	"testing"
)

func TestContextWithTraceID_RoundTrips(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "abc123")
	id, ok := TraceIDFromContext(ctx)
	if !ok || id != "abc123" {
		t.Errorf("TraceIDFromContext = (%q, %v), want (\"abc123\", true)", id, ok)
	}
}

func TestTraceIDFromContext_AbsentWhenNotSet(t *testing.T) {
	id, ok := TraceIDFromContext(context.Background())
	if ok || id != "" {
		t.Errorf("TraceIDFromContext = (%q, %v), want (\"\", false)", id, ok)
	}
}

func TestTraceIDFromContext_EmptyStringTreatedAsAbsent(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "")
	id, ok := TraceIDFromContext(ctx)
	if ok || id != "" {
		t.Errorf("TraceIDFromContext = (%q, %v), want (\"\", false)", id, ok)
	}
}

func TestTraceHeaders_EmptyWhenAbsent(t *testing.T) {
	got := TraceHeaders(context.Background())
	if len(got) != 0 {
		t.Errorf("TraceHeaders = %v, want empty map", got)
	}
}

func TestTraceHeaders_PopulatedWhenPresent(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	got := TraceHeaders(ctx)
	want := "00-0123456789abcdef0123456789abcdef-0000000000000000-01"
	if got["traceparent"] != want {
		t.Errorf("TraceHeaders[traceparent] = %q, want %q", got["traceparent"], want)
	}
}

func TestTraceHeaders_IncludesAmbientSpanID(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	ctx = ContextWithSpanID(ctx, "fedcba9876543210")
	got := TraceHeaders(ctx)
	want := "00-0123456789abcdef0123456789abcdef-fedcba9876543210-01"
	if got["traceparent"] != want {
		t.Errorf("TraceHeaders[traceparent] = %q, want %q", got["traceparent"], want)
	}
}

func TestValidTraceIDForEnvelope(t *testing.T) {
	cases := []struct {
		id   string
		want bool
	}{
		{"abc123", true},
		{"bad\x1eid", false},
		{"bad\nid", false},
	}
	for _, c := range cases {
		if got := validTraceIDForEnvelope(c.id); got != c.want {
			t.Errorf("validTraceIDForEnvelope(%q) = %v, want %v", c.id, got, c.want)
		}
	}
}
