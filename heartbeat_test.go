package weft

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestValidService(t *testing.T) {
	for _, ok := range []string{"a", "svc-a", "svc.b", "svc_c", "1234", strings.Repeat("a", 128)} {
		if !validService(ok) {
			t.Errorf("validService(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "with space", "with/slash", "semi;colon", strings.Repeat("a", 129)} {
		if validService(bad) {
			t.Errorf("validService(%q) = true, want false", bad)
		}
	}
}

func TestHeartbeat_EmitsRecordsOnRecordsSocket(t *testing.T) {
	// Heartbeats now ride the records transport: each beat is a
	// frame at app "<service>/heartbeats" on the same Unix socket
	// Send uses — no separate heartbeat socket.
	fs := newFakeSocket(t)
	c, err := New(WithSocket(fs.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go c.Heartbeat(ctx, "looper", 30*time.Millisecond)

	// Fires once immediately, then every 30ms.
	fs.waitForN(t, 2)
	cancel()

	got := fs.snapshot()[0]
	if !strings.HasPrefix(got, "looper/heartbeats:") {
		t.Errorf("frame = %q, want looper/heartbeats: prefix", got)
	}
	if !strings.Contains(got, `"type":"heartbeat"`) || !strings.Contains(got, `"service":"looper"`) {
		t.Errorf("frame body missing type/service: %q", got)
	}
	if !strings.Contains(got, `"ts":"`) {
		t.Errorf("frame missing ts: %q", got)
	}
}

func TestHeartbeat_InvalidServiceReturnsWithoutEmitting(t *testing.T) {
	fs := newFakeSocket(t)
	c, err := New(WithSocket(fs.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	done := make(chan struct{})
	go func() {
		c.Heartbeat(context.Background(), "bad name", 10*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Heartbeat blocked on an invalid service name")
	}
	if n := len(fs.snapshot()); n != 0 {
		t.Errorf("emitted %d frames for invalid service, want 0", n)
	}
}

func TestHeartbeat_NoOpWhenNoTransport(t *testing.T) {
	// No socket and no token → noop records transport. Heartbeat
	// should log once and return, not block or spam every tick.
	t.Setenv("WEFT_TOKEN", "")
	c, err := New(WithSocket(""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	done := make(chan struct{})
	go func() {
		c.Heartbeat(context.Background(), "svc", 10*time.Millisecond)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Heartbeat blocked despite no transport")
	}
}
