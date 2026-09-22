package weft

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestSlogHandler_AddsTraceIDWhenAmbient(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewSlogHandler(slog.NewJSONHandler(&buf, nil)))

	ctx := ContextWithTraceID(context.Background(), "abc123")
	logger.InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output not JSON: %v (raw %q)", err, buf.String())
	}
	if rec["trace_id"] != "abc123" {
		t.Errorf("trace_id = %v, want abc123", rec["trace_id"])
	}
}

func TestSlogHandler_NoTraceIDWhenAbsent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewSlogHandler(slog.NewJSONHandler(&buf, nil)))

	logger.Info("hello") // no context passed at all

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output not JSON: %v (raw %q)", err, buf.String())
	}
	if _, ok := rec["trace_id"]; ok {
		t.Errorf("trace_id = %v, want absent", rec["trace_id"])
	}
}

func TestSlogHandler_WithAttrsDelegates(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewSlogHandler(slog.NewJSONHandler(&buf, nil))).With("k", "v")

	ctx := ContextWithTraceID(context.Background(), "abc123")
	logger.InfoContext(ctx, "hello")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output not JSON: %v (raw %q)", err, buf.String())
	}
	if rec["k"] != "v" {
		t.Errorf("k = %v, want v (WithAttrs should still apply)", rec["k"])
	}
	if rec["trace_id"] != "abc123" {
		t.Errorf("trace_id = %v, want abc123", rec["trace_id"])
	}
}

func TestSlogHandler_WithGroupDelegates(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(NewSlogHandler(slog.NewJSONHandler(&buf, nil))).WithGroup("g")

	logger.Info("hello", "k", "v")

	var rec map[string]any
	if err := json.Unmarshal(buf.Bytes(), &rec); err != nil {
		t.Fatalf("output not JSON: %v (raw %q)", err, buf.String())
	}
	group, ok := rec["g"].(map[string]any)
	if !ok {
		t.Fatalf("rec[g] = %v, want a nested group object", rec["g"])
	}
	if group["k"] != "v" {
		t.Errorf("g.k = %v, want v (WithGroup should still apply)", group["k"])
	}
}
