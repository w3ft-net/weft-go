package weft

import (
	"errors"
	"strings"
	"testing"
)

func TestNew_NoSocketYieldsNoopTransport(t *testing.T) {
	// Empty socket path → noop transport. The Client should
	// construct (so importing the SDK without weftd never crashes
	// the host), but Send returns errTransportUnavailable so the
	// caller knows nothing is being delivered.
	c, err := New(WithSocket(""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = c.Send("anyapp", map[string]any{"x": 1})
	if !errors.Is(err, errTransportUnavailable) {
		t.Errorf("err = %v, want errTransportUnavailable", err)
	}
}

func TestNew_WithSocketSelectsSocketTransport(t *testing.T) {
	c, err := New(WithSocket("/tmp/whatever.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, ok := c.records.(*socketTransport); !ok {
		t.Errorf("records = %T, want *socketTransport", c.records)
	}
}

func TestSend_RejectsEmptyApp(t *testing.T) {
	c, err := New(WithSocket("/tmp/x.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Send("", map[string]any{"x": 1}); err == nil || !strings.Contains(err.Error(), "app is empty") {
		t.Errorf("err = %v, want 'app is empty'", err)
	}
}

func TestSend_MarshalsEvent(t *testing.T) {
	fs := newFakeSocket(t)
	c, err := New(WithSocket(fs.path), WithHeartbeatSocket(""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Send("billing", map[string]any{"event": "charge", "amount": 4200}); err != nil {
		t.Fatal(err)
	}
	fs.waitForN(t, 1)
	got := fs.snapshot()[0]
	if !strings.HasPrefix(got, "billing:") {
		t.Errorf("frame = %q, want billing: prefix", got)
	}
	if !strings.Contains(got, `"event":"charge"`) || !strings.Contains(got, `"amount":4200`) {
		t.Errorf("frame body missing fields: %q", got)
	}
}

func TestHeartbeat_DisabledWhenSocketEmpty(t *testing.T) {
	// Empty heartbeat socket should make Heartbeat a no-op that
	// returns immediately. Covered structurally in
	// TestClient_Heartbeat_NoOpWhenSocketDisabled — here we just
	// confirm the heartbeat field is unset so the no-op branch
	// can be reached.
	c, err := New(WithSocket("/tmp/x.sock"), WithHeartbeatSocket(""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.heartbeat != nil {
		t.Errorf("heartbeat = %v, want nil for empty socket", c.heartbeat)
	}
}
