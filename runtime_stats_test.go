package weft

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRuntimeStats_EmitsRecord(t *testing.T) {
	fs := newFakeSocket(t)
	c, err := New(WithSocket(fs.path), WithHeartbeatSocket(""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.RuntimeStats(ctx, "test-svc", 50*time.Millisecond)

	fs.waitForN(t, 1) // emit fires once immediately

	frame := fs.snapshot()[0]

	prefix := "test-svc/runtime:"
	if !strings.HasPrefix(frame, prefix) {
		t.Fatalf("frame = %q, want %q prefix", frame, prefix)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(strings.TrimPrefix(frame, prefix)), &payload); err != nil {
		t.Fatalf("payload not JSON: %v (raw %q)", err, frame)
	}
	if payload["type"] != "go_runtime" {
		t.Errorf("type = %v, want go_runtime", payload["type"])
	}
	if payload["service"] != "test-svc" {
		t.Errorf("service = %v, want test-svc", payload["service"])
	}
	for _, want := range []string{"goroutines", "heap_alloc_mb", "gc_count", "uptime_s"} {
		if _, ok := payload[want]; !ok {
			t.Errorf("payload missing %q", want)
		}
	}

	if g, ok := payload["goroutines"].(float64); !ok || int(g) < runtime.NumGoroutine()/2 {
		// JSON numbers come back as float64. Should be a positive
		// number around the current process's goroutine count.
		t.Errorf("goroutines = %v, want a sane positive integer", payload["goroutines"])
	}
}

func TestRuntimeStats_TickerCadence(t *testing.T) {
	fs := newFakeSocket(t)
	c, err := New(WithSocket(fs.path), WithHeartbeatSocket(""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.RuntimeStats(ctx, "svc", 30*time.Millisecond)

	// Immediate emit + at least 2 ticks within ~120ms.
	time.Sleep(120 * time.Millisecond)
	cancel()

	if got := len(fs.snapshot()); got < 3 {
		t.Errorf("emit count = %d, want ≥ 3 (immediate + ≥2 ticks)", got)
	}
}

func TestRuntimeStats_DefaultIntervalUsedForZero(t *testing.T) {
	// Non-positive interval should fall back to the default rather
	// than spin a zero-duration ticker. We can't wait the full
	// default interval in a unit test, but we can confirm the
	// immediate emit fires and no panic occurs.
	fs := newFakeSocket(t)
	c, err := New(WithSocket(fs.path), WithHeartbeatSocket(""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.RuntimeStats(ctx, "svc", 0)

	fs.waitForN(t, 1)
}

func TestPickRank(t *testing.T) {
	cases := []struct {
		n    int
		p    float64
		want int
	}{
		{0, 0.5, 0},
		{1, 0.5, 0},
		{2, 0.5, 0}, // floor(0.5 * 1) = 0
		{4, 0.5, 1}, // floor(0.5 * 3) = 1
		{4, 0.95, 2},
		{4, 0.99, 2},
		{100, 0.5, 49},
		{100, 0.95, 94},
		{100, 0.99, 98},
	}
	for _, c := range cases {
		if got := pickRank(c.n, c.p); got != c.want {
			t.Errorf("pickRank(%d, %.2f) = %d, want %d", c.n, c.p, got, c.want)
		}
	}
}
