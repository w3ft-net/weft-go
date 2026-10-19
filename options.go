package weft

import "os"

// Defaults for env-driven configuration.
const (
	DefaultRecordsSocket = "/var/run/weftd.sock"
	DefaultEndpoint      = "https://ingest.w3ft.net"

	// Deprecated: heartbeats now ride the records transport (see
	// Client.Heartbeat); there is no separate heartbeat socket.
	// Retained only so existing references compile.
	DefaultHeartbeatSocket = "/var/run/weftd-heartbeat.sock"
)

// Option configures a Client at construction time. Programmatic
// equivalents to the WEFT_* env vars; useful in tests and embedded
// scenarios.
type Option func(*config)

type config struct {
	recordsSocket string
	endpoint      string
	token         string
	// disableSocket forces the HTTP transport even when a socket
	// path resolves. WithHTTP sets this; useful for testing the
	// fallback or for processes inside a container that has weftd
	// but wants to bypass it.
	disableSocket bool
}

func defaultConfig() *config {
	c := &config{
		recordsSocket: firstNonEmpty(os.Getenv("WEFT_SOCKET"), DefaultRecordsSocket),
		endpoint:      firstNonEmpty(os.Getenv("WEFT_ENDPOINT"), DefaultEndpoint),
		token:         os.Getenv("WEFT_TOKEN"),
	}
	return c
}

// WithSocket overrides the records-socket path. Empty string
// disables the local socket transport (forces HTTP fallback).
func WithSocket(path string) Option {
	return func(c *config) { c.recordsSocket = path }
}

// WithHeartbeatSocket is a no-op retained for source compatibility.
//
// Deprecated: heartbeats now travel on the records transport (see
// Client.Heartbeat). There is no separate heartbeat socket, so this
// option does nothing. Use WithSocket/WithHTTP to configure the
// records transport that heartbeats share.
func WithHeartbeatSocket(string) Option {
	return func(*config) {}
}

// WithHTTP forces the HTTP transport for records and provides
// the bearer token. Useful in environments without local weftd
// (Lambda, serverless, dev boxes) or when bypassing weftd is
// desirable.
func WithHTTP(endpoint, token string) Option {
	return func(c *config) {
		c.endpoint = endpoint
		c.token = token
		c.disableSocket = true
	}
}

// WithToken sets the bearer token for HTTP-fallback transport
// without forcing HTTP. Lets a process succeed via the local
// socket and fall through to authenticated HTTP if the socket
// isn't reachable.
func WithToken(token string) Option {
	return func(c *config) { c.token = token }
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
