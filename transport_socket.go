package weft

import (
	"fmt"
	"net"
	"strings"
	"sync"
	"time"
)

// socketTransport writes app-prefixed lines to a Unix socket
// matching the wire format in weft's weftd.SocketListener:
//
//	<app>:<message>\n
//
// Reconnect-on-error: the transport lazily opens the connection
// and reopens it once on each write that fails, so a weftd
// restart doesn't require an SDK restart.
//
// Concurrency: a single mutex guards the connection. Future
// versions may batch through a goroutine + buffered channel; v1
// keeps the design as small as possible.
type socketTransport struct {
	path string

	mu   sync.Mutex
	conn net.Conn
}

func newSocketTransport(path string) *socketTransport {
	return &socketTransport{path: path}
}

func (t *socketTransport) sendLine(app, line string) error {
	if strings.ContainsRune(line, '\n') {
		// Newline-delimited framing relies on the line having no
		// embedded newlines. Reject explicitly rather than
		// silently splitting into two records.
		return fmt.Errorf("weft: line contains newline")
	}
	frame := app + ":" + line + "\n"

	t.mu.Lock()
	defer t.mu.Unlock()

	if t.conn == nil {
		c, err := t.dial()
		if err != nil {
			return err
		}
		t.conn = c
	}
	if _, err := t.conn.Write([]byte(frame)); err == nil {
		return nil
	}

	// First write failed — reconnect once and retry. This catches
	// the common "weftd restarted, our cached conn is stale" path
	// without retrying forever on a hard failure.
	t.conn.Close()
	c, err := t.dial()
	if err != nil {
		t.conn = nil
		return err
	}
	t.conn = c
	if _, err := t.conn.Write([]byte(frame)); err != nil {
		t.conn.Close()
		t.conn = nil
		return fmt.Errorf("weft: socket write after reconnect: %w", err)
	}
	return nil
}

func (t *socketTransport) close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.conn == nil {
		return nil
	}
	err := t.conn.Close()
	t.conn = nil
	return err
}

func (t *socketTransport) dial() (net.Conn, error) {
	c, err := net.DialTimeout("unix", t.path, 2*time.Second)
	if err != nil {
		return nil, fmt.Errorf("weft: dial unix:%s: %w", t.path, err)
	}
	return c, nil
}

var _ transport = (*socketTransport)(nil)
