package weft

import (
	"context"
	"regexp"
	"testing"
)

var hexSpanIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

func TestNewSpanID_ShapeAndUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id := NewSpanID()
		if !hexSpanIDRe.MatchString(id) {
			t.Fatalf("NewSpanID() = %q, want 16 lowercase hex chars", id)
		}
		if seen[id] {
			t.Fatalf("NewSpanID() produced a duplicate: %q", id)
		}
		seen[id] = true
	}
}

func TestStartSpan_NoopWithoutAmbientTraceID(t *testing.T) {
	lines := captureLines(t, func() {
		ctx, span := StartSpan(context.Background(), "test.op")
		if _, ok := CurrentSpanID(ctx); ok {
			t.Errorf("StartSpan without a trace ID should not establish an ambient span ID")
		}
		span.RecordError(errBoom)
		span.End()
	})
	if len(lines) != 0 {
		t.Errorf("StartSpan without an ambient trace ID logged %d lines, want none", len(lines))
	}
}

func TestStartSpan_LogsStartAndEnd(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	lines := captureLines(t, func() {
		_, span := StartSpan(ctx, "svc.Handle")
		span.End()
	})
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (start + done)", len(lines))
	}
	start, done := lines[0], lines[1]
	if start.body["msg"] != "span start" || done.body["msg"] != "span done" {
		t.Errorf("msgs = %v, %v", start.body["msg"], done.body["msg"])
	}
	for _, l := range lines {
		if l.traceID != "0123456789abcdef0123456789abcdef" || l.spanName != "svc.Handle" || !hexSpanIDRe.MatchString(l.spanID) {
			t.Errorf("header = %+v, want trace, span name and a hex span ID", l)
		}
		if l.parentID != "" {
			t.Errorf("parent = %q, want none for a root span", l.parentID)
		}
	}
	if done.durNs == "" || done.durNs == "0" {
		t.Errorf("done duration = %q, want nanoseconds", done.durNs)
	}
	if done.body["status"] != "ok" {
		t.Errorf("status = %v, want ok (no RecordError was called)", done.body["status"])
	}
	for _, k := range []string{"span_name", "span_id", "duration_ns", "trace_id"} {
		if _, ok := done.body[k]; ok {
			t.Errorf("body repeats %q, which the header carries", k)
		}
	}
}

func TestSpan_RecordErrorMarksDoneLineWithoutEnding(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	lines := captureLines(t, func() {
		_, span := StartSpan(ctx, "svc.Query")
		span.RecordError(errBoom)
		// A span that records an error can still keep going (e.g. a
		// retry loop) and end successfully overall: RecordError marks
		// the done line, it doesn't end the span itself.
		span.End()
	})
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want 3 (start, error, done)", len(lines))
	}
	if lines[1].body["msg"] != "span error" || lines[1].body["err"] != "boom" || lines[1].level != "ERROR" {
		t.Errorf("error line = %+v", lines[1])
	}
	if lines[2].body["status"] != "error" {
		t.Errorf("done status = %v, want error after RecordError", lines[2].body["status"])
	}
}

func TestSpan_RecordErrorNilIsNoop(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	lines := captureLines(t, func() {
		_, span := StartSpan(ctx, "api.access")
		span.RecordError(nil)
		span.End()
	})
	if len(lines) != 2 || lines[1].body["status"] != "ok" {
		t.Fatalf("got %d lines, want start + done with status ok", len(lines))
	}
}

func TestStartSpan_EndIsIdempotent(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	lines := captureLines(t, func() {
		_, span := StartSpan(ctx, "api.access")
		span.End()
		span.End()
		span.End()
	})
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2 (start + exactly one done)", len(lines))
	}
}

func TestStartSpan_NestedSpanParentsOffCaller(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	lines := captureLines(t, func() {
		outerCtx, outer := StartSpan(ctx, "outer")
		_, inner := StartSpan(outerCtx, "inner")
		inner.End()
		outer.End()
	})
	if len(lines) != 4 {
		t.Fatalf("got %d lines, want 4 (outer start, inner start, inner done, outer done)", len(lines))
	}
	outerID, innerParent := lines[0].spanID, lines[1].parentID
	if outerID == "" || outerID != innerParent {
		t.Errorf("inner parent = %q, want outer span ID %q", innerParent, outerID)
	}
}

var errBoom = boomErr{}

type boomErr struct{}

func (boomErr) Error() string { return "boom" }
