package weft

import (
	"bufio"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeSocket runs a tiny Unix-socket server that reads
// newline-delimited frames and stashes them. Mimics the wire
// shape weftd's SocketListener accepts.
type fakeSocket struct {
	path string
	ln   net.Listener

	mu       sync.Mutex
	lines    []string
	accepted []net.Conn
}

func newFakeSocket(t *testing.T) *fakeSocket {
	t.Helper()
	f, err := os.CreateTemp("/tmp", "weftgo-test-*.sock")
	if err != nil {
		t.Fatal(err)
	}
	path := f.Name()
	f.Close()
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })

	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })

	fs := &fakeSocket{path: path, ln: ln}
	go fs.acceptLoop()
	return fs
}

func (f *fakeSocket) acceptLoop() {
	for {
		c, err := f.ln.Accept()
		if err != nil {
			return
		}
		f.mu.Lock()
		f.accepted = append(f.accepted, c)
		f.mu.Unlock()
		go f.handle(c)
	}
}

// closeAccepted force-closes every connection the server has
// accepted so far. The SDK's cached conn will return EOF/EPIPE
// on its next write, exercising the reconnect path.
func (f *fakeSocket) closeAccepted() {
	f.mu.Lock()
	conns := append([]net.Conn(nil), f.accepted...)
	f.accepted = nil
	f.mu.Unlock()
	for _, c := range conns {
		c.Close()
	}
}

func (f *fakeSocket) handle(c net.Conn) {
	defer c.Close()
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		f.mu.Lock()
		f.lines = append(f.lines, sc.Text())
		f.mu.Unlock()
	}
}

func (f *fakeSocket) snapshot() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.lines...)
}

func (f *fakeSocket) waitForN(t *testing.T, n int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(f.snapshot()) >= n {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d lines; got %v", n, f.snapshot())
}

func TestSocketTransport_SendsFrames(t *testing.T) {
	fs := newFakeSocket(t)
	tr := newSocketTransport(fs.path)
	defer tr.close()

	if err := tr.sendLine("myapp", `{"x":1}`); err != nil {
		t.Fatal(err)
	}
	if err := tr.sendLine("myapp", `{"x":2}`); err != nil {
		t.Fatal(err)
	}
	fs.waitForN(t, 2)

	got := fs.snapshot()
	want := []string{"myapp:" + `{"x":1}`, "myapp:" + `{"x":2}`}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("frames = %v, want %v", got, want)
	}
}

func TestSocketTransport_RejectsEmbeddedNewline(t *testing.T) {
	fs := newFakeSocket(t)
	tr := newSocketTransport(fs.path)
	defer tr.close()

	if err := tr.sendLine("myapp", "line1\nline2"); err == nil {
		t.Error("expected error for embedded newline")
	}
}

func TestSocketTransport_ReconnectsAfterBrokenConn(t *testing.T) {
	// Send one frame, force-close the server's accepted conn (so
	// the SDK's cached conn returns EOF/EPIPE on the next write),
	// then send another. The reconnect path should re-dial
	// silently and the second frame should land via the new conn.
	fs := newFakeSocket(t)
	tr := newSocketTransport(fs.path)
	defer tr.close()

	if err := tr.sendLine("myapp", "first"); err != nil {
		t.Fatal(err)
	}
	fs.waitForN(t, 1)

	fs.closeAccepted()

	if err := tr.sendLine("myapp", "second"); err != nil {
		t.Fatalf("send after reconnect: %v", err)
	}
	fs.waitForN(t, 2)
	got := fs.snapshot()
	if !strings.HasSuffix(got[len(got)-1], ":second") {
		t.Errorf("frames = %v, want trailing :second", got)
	}
}

func TestSocketTransport_DialFailureReturnsError(t *testing.T) {
	// Pointing at a non-existent socket should fail predictably.
	tr := newSocketTransport("/tmp/this-socket-definitely-does-not-exist-weftgo-test")
	defer tr.close()
	if err := tr.sendLine("a", "b"); err == nil {
		t.Error("expected error sending to non-existent socket")
	}
}
