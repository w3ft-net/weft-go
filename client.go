package weft

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// Client is the typed handle for sending records and heartbeats.
// Safe for concurrent use. Construct with New and call Close on
// shutdown.
type Client struct {
	cfg *config

	records transport

	closeOnce sync.Once
}

// New constructs a Client from env defaults plus any options.
// Construction never blocks on a network call: the records
// transport is selected and stored, but the underlying connection
// is opened lazily on the first Send. So a process that imports
// the SDK but never sends anything pays nothing.
//
// Selection rule for records: if WithHTTP is in effect, use HTTP.
// Otherwise prefer the Unix socket if its path is non-empty;
// fall back to HTTP if a token is set; otherwise use a no-op
// transport (Send returns errTransportUnavailable).
func New(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, o := range opts {
		o(cfg)
	}

	c := &Client{cfg: cfg}
	c.records = pickRecordsTransport(cfg)
	return c, nil
}

func pickRecordsTransport(cfg *config) transport {
	if cfg.disableSocket {
		if cfg.token == "" {
			return noopTransport{}
		}
		return newHTTPTransport(cfg.endpoint, cfg.token)
	}
	if cfg.recordsSocket != "" {
		// Don't probe the socket here — dial happens lazily on
		// the first send. A process that's started before weftd
		// is running should still construct successfully.
		return newSocketTransport(cfg.recordsSocket)
	}
	if cfg.token != "" {
		return newHTTPTransport(cfg.endpoint, cfg.token)
	}
	return noopTransport{}
}

// Send queues one record for the named app. event is anything
// json-marshalable; in v1 the marshaling and write are
// synchronous, so Send returns the transport error if any. A
// later version may add in-process batching.
//
// If ctx carries an ambient trace ID (see ContextWithTraceID,
// TraceMiddleware), it's attached to the record so it can be
// correlated with other log lines from the same request via
// `trace <id>`. Pass context.Background() when there's no request
// context to thread through.
func (c *Client) Send(ctx context.Context, app string, event any) error {
	if app == "" {
		return errors.New("weft: app is empty")
	}
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("weft: marshal: %w", err)
	}
	return c.records.sendLine(ctx, app, string(data))
}

// SendLine emits an already-serialized line as-is. Use when the
// caller has constructed JSON itself or wants to ship a
// non-JSON line that downstream parsers handle. See Send for ctx's
// role in trace-ID attachment.
func (c *Client) SendLine(ctx context.Context, app, line string) error {
	if app == "" {
		return errors.New("weft: app is empty")
	}
	return c.records.sendLine(ctx, app, line)
}

// DefaultHeartbeatInterval is the recommended heartbeat cadence.
const DefaultHeartbeatInterval = 30 * time.Second

// Heartbeat emits a liveness record for the named service every
// interval until ctx is cancelled. Heartbeats ride the same records
// transport as Send — each lands at app "<service>/heartbeats", i.e.
// the pseudofile /log/<namespace>/<service>/heartbeats.log — so no
// second socket is involved and heartbeats work over the HTTP
// transport too. Liveness is judged by recency: a service that stops
// emitting these rows is absent (see the weft::is_absent predicate).
//
// Typical usage in main():
//
//	go c.Heartbeat(ctx, "my-service", weft.DefaultHeartbeatInterval)
//
// interval ≤ 0 falls back to DefaultHeartbeatInterval. Send errors
// (transient socket hiccups, transient 5xx) are logged to stderr but
// don't terminate the loop. If no records transport is available (no
// socket and no token) the call logs once and returns — no log spam
// from a misconfigured client.
func (c *Client) Heartbeat(ctx context.Context, service string, interval time.Duration) {
	c.HeartbeatWithFields(ctx, service, interval, nil)
}

// HeartbeatWithFields behaves like Heartbeat but merges extra into
// every emitted record — e.g. an instance identifier so multiple
// processes running the same service are distinguishable downstream.
// The reserved keys (type, service, ts) always win over anything in
// extra, protecting the liveness contract other tooling depends on.
func (c *Client) HeartbeatWithFields(ctx context.Context, service string, interval time.Duration, extra map[string]any) {
	if !validService(service) {
		fmt.Fprintf(os.Stderr, "weft: heartbeat: invalid service name %q\n", service)
		return
	}
	if _, ok := c.records.(noopTransport); ok {
		fmt.Fprintf(os.Stderr, "weft: heartbeat %q: no transport (set WEFT_SOCKET or a token)\n", service)
		return
	}
	if interval <= 0 {
		interval = DefaultHeartbeatInterval
	}
	app := service + "/heartbeats"
	beat := func() {
		ev := make(map[string]any, len(extra)+3)
		for k, v := range extra {
			ev[k] = v
		}
		ev["type"] = "heartbeat"
		ev["service"] = service
		ev["ts"] = time.Now().UTC().Format(time.RFC3339Nano)
		if err := c.Send(ctx, app, ev); err != nil {
			fmt.Fprintf(os.Stderr, "weft: heartbeat %q: %v\n", service, err)
		}
	}
	// Fire once immediately so liveness lands without waiting an
	// interval.
	beat()
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			beat()
		}
	}
}

// Close releases resources. Safe to call multiple times. Doesn't
// stop Heartbeat or RuntimeStats goroutines — those are
// caller-scoped via their ctx.
func (c *Client) Close() error {
	var err error
	c.closeOnce.Do(func() {
		if c.records != nil {
			err = c.records.close()
		}
	})
	return err
}
