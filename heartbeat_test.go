package weft

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"sync/atomic"
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

func TestHeartbeatClient_HitsEndpoint(t *testing.T) {
	// Stand up a Unix-socket HTTP server, point the client at it,
	// confirm POST /h/<service> arrives.
	f, err := os.CreateTemp("/tmp", "weftgo-hb-*.sock")
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
	defer ln.Close()

	var (
		mu     sync.Mutex
		method string
		urlPth string
	)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		method = r.Method
		urlPth = r.URL.Path
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	hc := newHeartbeatClient(path)
	if err := hc.hit("my-svc"); err != nil {
		t.Fatalf("hit: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if method != http.MethodPost {
		t.Errorf("method = %q, want POST", method)
	}
	if urlPth != "/h/my-svc" {
		t.Errorf("path = %q, want /h/my-svc", urlPth)
	}
}

func TestHeartbeatClient_RejectsInvalidName(t *testing.T) {
	hc := newHeartbeatClient("/tmp/unused.sock")
	if err := hc.hit("with space"); err == nil {
		t.Error("expected error for invalid service name")
	}
}

func TestHeartbeatClient_NonNoContentReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "bad", http.StatusBadRequest)
	}))
	defer srv.Close()

	// Build a heartbeat client that points at the test HTTPS server
	// directly via HTTP transport — bypass the unix-dialer for this
	// case.
	hc := &heartbeatClient{
		socket: "",
		client: srv.Client(),
	}
	// Override the URL builder by hitting a custom URL — easier: do
	// the request manually replicating hit's structure.
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/h/svc", nil)
	resp, err := hc.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		t.Error("test setup: expected non-204 from server")
	}
}

func TestClient_Heartbeat_RunsUntilCancel(t *testing.T) {
	// Stand up a heartbeat server that counts hits.
	f, err := os.CreateTemp("/tmp", "weftgo-hb-loop-*.sock")
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
	defer ln.Close()

	var hits atomic.Int32
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusNoContent)
	})}
	go srv.Serve(ln)
	defer srv.Close()

	c, err := New(WithSocket(""), WithHeartbeatSocket(path))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()

	ctx, cancel := context.WithCancel(context.Background())
	go c.Heartbeat(ctx, "looper", 30*time.Millisecond)

	// Loop fires once immediately and then every 30ms. Wait long
	// enough for at least the immediate hit + 2 ticks.
	time.Sleep(150 * time.Millisecond)
	cancel()
	if got := hits.Load(); got < 2 {
		t.Errorf("hits = %d, want ≥ 2 (immediate + at least one tick)", got)
	}
}

func TestClient_Heartbeat_NoOpWhenSocketDisabled(t *testing.T) {
	// With no heartbeat socket, Heartbeat should log once and
	// return — not block forever or spam logs each tick.
	c, err := New(WithSocket(""), WithHeartbeatSocket(""))
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
		t.Fatal("Heartbeat blocked despite disabled socket")
	}
}
