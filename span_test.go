package weft

import (
	"bytes"
	"context"
	"log"
	"regexp"
	"strings"
	"testing"
)

var hexSpanIDRe = regexp.MustCompile(`^[0-9a-f]{16}$`)

func captureLog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	orig := log.Writer()
	origFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(orig)
		log.SetFlags(origFlags)
	}()
	fn()
	return buf.String()
}

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
	out := captureLog(t, func() {
		ctx, span := StartSpan(context.Background(), "test.op")
		if _, ok := CurrentSpanID(ctx); ok {
			t.Errorf("StartSpan without a trace ID should not establish an ambient span ID")
		}
		span.RecordError(errBoom)
		span.End()
	})
	if out != "" {
		t.Errorf("StartSpan without an ambient trace ID logged %q, want nothing", out)
	}
}

func TestStartSpan_LogsStartAndEnd(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	var out string
	out = captureLog(t, func() {
		_, span := StartSpan(ctx, "coordinator.Execute")
		span.End()
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2 (start + done):\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "span start name=coordinator.Execute trace_id=0123456789abcdef0123456789abcdef span_id=") {
		t.Errorf("start line = %q, wrong shape", lines[0])
	}
	if !strings.Contains(lines[0], "parent_span_id=") {
		t.Errorf("start line = %q, missing parent_span_id", lines[0])
	}
	if !strings.HasPrefix(lines[1], "span done name=coordinator.Execute trace_id=0123456789abcdef0123456789abcdef span_id=") {
		t.Errorf("done line = %q, wrong shape", lines[1])
	}
	if !strings.Contains(lines[1], "duration=") {
		t.Errorf("done line = %q, missing duration", lines[1])
	}
	if !strings.Contains(lines[1], "status=ok") {
		t.Errorf("done line = %q, want status=ok (no RecordError was called)", lines[1])
	}
}

func TestSpan_RecordErrorMarksDoneLineWithoutEnding(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	out := captureLog(t, func() {
		_, span := StartSpan(ctx, "shard.Query")
		span.RecordError(errBoom)
		// A span that records an error can still keep going (e.g. a
		// retry loop) and end successfully overall — RecordError marks
		// the done line, it doesn't end the span itself.
		span.End()
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d log lines, want 3 (start, error, done):\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[1], "span error name=shard.Query") || !strings.Contains(lines[1], "err=boom") {
		t.Errorf("error line = %q, wrong shape", lines[1])
	}
	if !strings.Contains(lines[2], "status=error") {
		t.Errorf("done line = %q, want status=error after RecordError", lines[2])
	}
}

func TestSpan_RecordErrorNilIsNoop(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	out := captureLog(t, func() {
		_, span := StartSpan(ctx, "api.access")
		span.RecordError(nil)
		span.End()
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2 (start + done, RecordError(nil) logs nothing):\n%s", len(lines), out)
	}
	if !strings.Contains(lines[1], "status=ok") {
		t.Errorf("done line = %q, want status=ok (RecordError(nil) is a no-op)", lines[1])
	}
}

func TestStartSpan_EndIsIdempotent(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	out := captureLog(t, func() {
		_, span := StartSpan(ctx, "api.access")
		span.End()
		span.End()
		span.End()
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("got %d log lines, want 2 (start + exactly one done, despite 3 End calls):\n%s", len(lines), out)
	}
}

func TestStartSpan_NestedSpanParentsOffCaller(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "0123456789abcdef0123456789abcdef")
	out := captureLog(t, func() {
		outerCtx, outer := StartSpan(ctx, "outer")
		_, inner := StartSpan(outerCtx, "inner")
		inner.End()
		outer.End()
	})
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("got %d log lines, want 4 (outer start, inner start, inner done, outer done):\n%s", len(lines), out)
	}
	outerID := mustFieldValue(t, lines[0], "span_id")
	innerParentID := mustFieldValue(t, lines[1], "parent_span_id")
	if outerID == "" || innerParentID == "" {
		t.Fatal("failed to extract span IDs from log output")
	}
	if outerID != innerParentID {
		t.Errorf("inner span's parent_span_id = %q, want outer span's span_id %q", innerParentID, outerID)
	}
}

var errBoom = boomErr{}

type boomErr struct{}

func (boomErr) Error() string { return "boom" }

func mustFieldValue(t *testing.T, line, field string) string {
	t.Helper()
	idx := strings.Index(line, field+"=")
	if idx == -1 {
		t.Fatalf("line %q has no field %q", line, field)
	}
	rest := line[idx+len(field)+1:]
	if sp := strings.IndexByte(rest, ' '); sp != -1 {
		return rest[:sp]
	}
	return rest
}
