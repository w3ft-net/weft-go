package weft

import (
	"context"
	"testing"
)

func TestInfofWarnfErrorf(t *testing.T) {
	ctx := ContextWithTraceID(context.Background(), "abc")
	lines := captureLines(t, func() {
		Infof(ctx, "done lines=%d", 10)
		Warnf(ctx, "slow %s", "query")
		Errorf(ctx, "failed: %v", "boom")
	})
	want := []struct{ level, msg string }{{"INFO", "done lines=10"}, {"WARN", "slow query"}, {"ERROR", "failed: boom"}}
	if len(lines) != 3 {
		t.Fatalf("got %d lines", len(lines))
	}
	for i, w := range want {
		if lines[i].level != w.level || lines[i].body["msg"] != w.msg || lines[i].traceID != "abc" {
			t.Errorf("line %d = %+v, want %s %q with trace abc", i, lines[i], w.level, w.msg)
		}
	}
}
