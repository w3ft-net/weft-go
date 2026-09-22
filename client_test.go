package weft

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNew_NoCredentialsYieldsNoopTransport(t *testing.T) {
	// No env, no options. The Client should construct (so importing
	// the SDK in a context without weftd never crashes the host),
	// but Send should fail with errTransportUnavailable so the
	// caller knows nothing is being delivered.
	c, err := New(WithSocket(""), WithHTTP("", ""))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	err = c.Send(context.Background(), "anyapp", map[string]any{"x": 1})
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

func TestNew_WithHTTPForcesHTTPTransport(t *testing.T) {
	// WithHTTP must override even when a socket path is configured,
	// matching the docstring "forces the HTTP transport".
	c, err := New(WithSocket("/tmp/whatever.sock"), WithHTTP("https://x.test", "tok"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, ok := c.records.(*httpTransport); !ok {
		t.Errorf("records = %T, want *httpTransport (WithHTTP should win)", c.records)
	}
}

func TestSend_RejectsEmptyApp(t *testing.T) {
	c, err := New(WithSocket("/tmp/x.sock"))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if err := c.Send(context.Background(), "", map[string]any{"x": 1}); err == nil || !strings.Contains(err.Error(), "app is empty") {
		t.Errorf("err = %v, want 'app is empty'", err)
	}
}

func TestSend_MarshalsEvent(t *testing.T) {
	fs := newFakeSocket(t)
	c, err := New(WithSocket(fs.path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	if err := c.Send(context.Background(), "billing", map[string]any{"event": "charge", "amount": 4200}); err != nil {
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

